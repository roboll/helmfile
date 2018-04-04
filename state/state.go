package state

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/roboll/helmfile/helmexec"

	"bytes"
	"path"
	"regexp"

	yaml "gopkg.in/yaml.v1"
)

type HelmState struct {
	BaseChartPath      string
	Context            string           `yaml:"context"`
	DeprecatedReleases []ReleaseSpec    `yaml:"charts"`
	Namespace          string           `yaml:"namespace"`
	Repositories       []RepositorySpec `yaml:"repositories"`
	Releases           []ReleaseSpec    `yaml:"releases"`
}

type RepositorySpec struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

type ReleaseSpec struct {
	// Chart is the name of the chart being installed to create this release
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	Verify  bool   `yaml:"verify"`

	// Name is the name of this release
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
	Values    []string          `yaml:"values"`
	Secrets   []string          `yaml:"secrets"`
	SetValues []SetValue        `yaml:"set"`

	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
	EnvValues []SetValue `yaml:"env"`

	// generatedValues are values that need cleaned up on exit
	generatedValues []string
}

type SetValue struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

func ReadFromFile(file string) (*HelmState, error) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return readFromYaml(content, file)
}

func readFromYaml(content []byte, file string) (*HelmState, error) {
	var state HelmState

	state.BaseChartPath, _ = filepath.Abs(path.Dir(file))
	if err := yaml.Unmarshal(content, &state); err != nil {
		return nil, err
	}

	if len(state.DeprecatedReleases) > 0 {
		if len(state.Releases) > 0 {
			return nil, fmt.Errorf("failed to parse %s: you can't specify both `charts` and `releases` sections", file)
		}
		state.Releases = state.DeprecatedReleases
		state.DeprecatedReleases = []ReleaseSpec{}
	}

	return &state, nil
}

var stringTemplateFuncMap = template.FuncMap{
	"env": getEnvVar,
}

func stringTemplate() *template.Template {
	return template.New("stringTemplate").Funcs(stringTemplateFuncMap)
}

func getEnvVar(envVarName string) (string, error) {
	envVarValue, isSet := os.LookupEnv(envVarName)

	if !isSet {
		errMsg := fmt.Sprintf("Environment Variable '%s' is not set. Please make sure it is set and try again.", envVarName)
		return "", errors.New(errMsg)
	}

	return envVarValue, nil
}

func renderTemplateString(s string) (string, error) {
	var t, parseErr = stringTemplate().Parse(s)
	if parseErr != nil {
		return "", parseErr
	}

	var tplString bytes.Buffer
	var execErr = t.Execute(&tplString, nil)

	if execErr != nil {
		return "", execErr
	}

	return tplString.String(), nil
}

func (state *HelmState) applyDefaultsTo(spec *ReleaseSpec) {
	if state.Namespace != "" {
		spec.Namespace = state.Namespace
	}
}

func (state *HelmState) SyncRepos(helm helmexec.Interface) []error {
	errs := []error{}

	for _, repo := range state.Repositories {
		url, err := renderTemplateString(repo.URL)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := helm.AddRepo(repo.Name, url, repo.CertFile, repo.KeyFile); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	if err := helm.UpdateRepo(); err != nil {
		return []error{err}
	}
	return nil
}

func (state *HelmState) SyncReleases(helm helmexec.Interface, additionalValues []string, workerLimit int) []error {
	errs := []error{}
	jobQueue := make(chan *ReleaseSpec)
	doneQueue := make(chan bool)
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}
	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				state.applyDefaultsTo(release)
				flags, flagsErr := flagsForRelease(helm, state.BaseChartPath, release)
				if flagsErr != nil {
					errQueue <- flagsErr
					doneQueue <- true
					continue
				}

				haveValueErr := false
				for _, value := range additionalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errQueue <- err
						haveValueErr = true
					}
					flags = append(flags, "--values", valfile)
				}

				if haveValueErr {
					doneQueue <- true
					continue
				}

				chart := normalizeChart(state.BaseChartPath, release.Chart)
				if err := helm.SyncRelease(release.Name, chart, flags...); err != nil {
					errQueue <- err
				}
				doneQueue <- true
			}
		}()
	}

	go func() {
		for i := 0; i < len(state.Releases); i++ {
			jobQueue <- &state.Releases[i]
		}
		close(jobQueue)
	}()

	for i := 0; i < len(state.Releases); {
		select {
		case err := <-errQueue:
			errs = append(errs, err)
		case <-doneQueue:
			i++
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (state *HelmState) DiffReleases(helm helmexec.Interface, additionalValues []string) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for i := 0; i < len(state.Releases); i++ {
		release := &state.Releases[i]
		wg.Add(1)
		go func(wg *sync.WaitGroup, release *ReleaseSpec) {
			// Plugin command doesn't support explicit namespace
			release.Namespace = ""
			flags, flagsErr := flagsForRelease(helm, state.BaseChartPath, release)
			if flagsErr != nil {
				errs = append(errs, flagsErr)
			}

			for _, value := range additionalValues {
				valfile, err := filepath.Abs(value)
				if err != nil {
					errs = append(errs, err)
				}
				flags = append(flags, "--values", valfile)
			}
			if len(errs) == 0 {
				if err := helm.DiffRelease(release.Name, normalizeChart(state.BaseChartPath, release.Chart), flags...); err != nil {
					errs = append(errs, err)
				}
			}
			wg.Done()
		}(&wg, release)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (state *HelmState) DeleteReleases(helm helmexec.Interface) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, release := range state.Releases {
		wg.Add(1)
		go func(wg *sync.WaitGroup, release ReleaseSpec) {
			if err := helm.DeleteRelease(release.Name); err != nil {
				errs = append(errs, err)
			}
			wg.Done()
		}(&wg, release)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// Clean will remove any generated secrets
func (state *HelmState) Clean() []error {
	errs := []error{}

	for _, release := range state.Releases {
		for _, value := range release.generatedValues {
			err := os.Remove(value)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// FilterReleases allows for the execution of helm commands against a subset of the releases in the helmfile.
func (state *HelmState) FilterReleases(labels []string) error {
	var filteredReleases []ReleaseSpec
	releaseSet := map[string]ReleaseSpec{}
	filters := []ReleaseFilter{}
	for _, label := range labels {
		f, err := ParseLabels(label)
		if err != nil {
			return err
		}
		filters = append(filters, f)
	}
	for _, r := range state.Releases {
		if r.Labels == nil {
			r.Labels = map[string]string{}
		}
		// Let the release name be used as a tag
		r.Labels["name"] = r.Name
		for _, f := range filters {
			if r.Labels == nil {
				r.Labels = map[string]string{}
			}
			if f.Match(r) {
				releaseSet[r.Name] = r
				continue
			}
		}
	}
	for _, r := range releaseSet {
		filteredReleases = append(filteredReleases, r)
	}
	state.Releases = filteredReleases
	return nil
}

func (state *HelmState) UpdateDeps(helm helmexec.Interface) []error {
	errs := []error{}

	for _, release := range state.Releases {
		if isLocalChart(release.Chart) {
			if err := helm.UpdateDeps(normalizeChart(state.BaseChartPath, release.Chart)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) != 0 {
		return errs
	}
	return nil
}

// normalizeChart allows for the distinction between a file path reference and repository references.
// - Any single (or double character) followed by a `/` will be considered a local file reference and
// 	 be constructed relative to the `base path`.
// - Everything else is assumed to be an absolute path or an actual <repository>/<chart> reference.
func normalizeChart(basePath, chart string) string {
	if !isLocalChart(chart) {
		return chart
	}
	return filepath.Join(basePath, chart)
}

func isLocalChart(chart string) bool {
	regex, _ := regexp.Compile("^[.]?./")
	return regex.MatchString(chart)
}

func flagsForRelease(helm helmexec.Interface, basePath string, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}
	if release.Verify {
		flags = append(flags, "--verify")
	}
	if release.Namespace != "" {
		flags = append(flags, "--namespace", release.Namespace)
	}
	for _, value := range release.Values {
		valfile := filepath.Join(basePath, value)
		valfileRendered, err := renderTemplateString(valfile)
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--values", valfileRendered)
	}
	for _, value := range release.Secrets {
		valfile := filepath.Join(basePath, value)
		path, err := renderTemplateString(valfile)
		if err != nil {
			return nil, err
		}

		valfileRendered, err := helm.DecryptSecret(path)
		if err != nil {
			return nil, err
		}

		release.generatedValues = append(release.generatedValues, valfileRendered)
		flags = append(flags, "--values", valfileRendered)
	}
	if len(release.SetValues) > 0 {
		val := []string{}
		for _, set := range release.SetValues {
			renderedValue, err := renderTemplateString(set.Value)
			if err != nil {
				return nil, err
			}
			val = append(val, fmt.Sprintf("%s=%s", set.Name, renderedValue))
		}
		flags = append(flags, "--set", strings.Join(val, ","))
	}

	/***********
	 * START 'env' section for backwards compatibility
	 ***********/
	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
	if len(release.EnvValues) > 0 {
		val := []string{}
		envValErrs := []string{}
		for _, set := range release.EnvValues {
			value, isSet := os.LookupEnv(set.Value)
			if isSet {
				val = append(val, fmt.Sprintf("%s=%s", set.Name, value))
			} else {
				errMsg := fmt.Sprintf("\t%s", set.Value)
				envValErrs = append(envValErrs, errMsg)
			}
		}
		if len(envValErrs) != 0 {
			joinedEnvVals := strings.Join(envValErrs, "\n")
			errMsg := fmt.Sprintf("Environment Variables not found. Please make sure they are set and try again:\n%s", joinedEnvVals)
			return nil, errors.New(errMsg)
		}
		flags = append(flags, "--set", strings.Join(val, ","))
	}
	/**************
	 * END 'env' section for backwards compatibility
	 **************/

	return flags, nil
}
