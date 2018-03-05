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
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type ReleaseSpec struct {
	// Chart is the name of the chart being installed to create this release
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	Verify  bool   `yaml:"verify"`

	// Name is the name of this release
	Name      string     `yaml:"name"`
	Namespace string     `yaml:"namespace"`
	Values    []string   `yaml:"values"`
	SetValues []SetValue `yaml:"set"`

	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
	EnvValues []SetValue `yaml:"env"`
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

func (state *HelmState) applyDefaultsTo(spec ReleaseSpec) ReleaseSpec {
	if state.Namespace != "" {
		spec.Namespace = state.Namespace
	}
	return spec
}

func (state *HelmState) SyncRepos(helm helmexec.Interface) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, repo := range state.Repositories {
		wg.Add(1)
		go func(wg *sync.WaitGroup, name, url string) {
			if err := helm.AddRepo(name, url); err != nil {
				errs = append(errs, err)
			}
			wg.Done()
		}(&wg, repo.Name, repo.URL)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	if err := helm.UpdateRepo(); err != nil {
		return []error{err}
	}
	return nil
}

func (state *HelmState) SyncReleases(helm helmexec.Interface, additonalValues []string, workerLimit int) []error {
	errs := []error{}
	jobQueue := make(chan ReleaseSpec)
	doneQueue := make(chan bool)
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}
	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				releaseWithDefaults := state.applyDefaultsTo(release)
				flags, flagsErr := flagsForRelease(state.BaseChartPath, &releaseWithDefaults)
				if flagsErr != nil {
					errQueue <- flagsErr
					doneQueue <- true
					continue
				}

				haveValueErr := false
				for _, value := range additonalValues {
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

				if err := helm.SyncRelease(release.Name, normalizeChart(state.BaseChartPath, release.Chart), flags...); err != nil {
					errQueue <- err
				}
				doneQueue <- true
			}
		}()
	}

	go func() {
		for _, release := range state.Releases {
			jobQueue <- release
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

func (state *HelmState) DiffReleases(helm helmexec.Interface, additonalValues []string) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, release := range state.Releases {
		wg.Add(1)
		go func(wg *sync.WaitGroup, release ReleaseSpec) {
			// Plugin command doesn't support explicit namespace
			release.Namespace = ""
			flags, flagsErr := flagsForRelease(state.BaseChartPath, &release)
			if flagsErr != nil {
				errs = append(errs, flagsErr)
			}
			for _, value := range additonalValues {
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

// normalizeChart allows for the distinction between a file path reference and repository references.
// - Any single (or double character) followed by a `/` will be considered a local file reference and
// 	 be constructed relative to the `base path`.
// - Everything else is assumed to be an absolute path or an actual <repository>/<chart> reference.
func normalizeChart(basePath, chart string) string {
	regex, _ := regexp.Compile("^[.]?./")
	if !regex.MatchString(chart) {
		return chart
	}
	return filepath.Join(basePath, chart)
}

func flagsForRelease(basePath string, release *ReleaseSpec) ([]string, error) {
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
