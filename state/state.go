package state

import (
	"errors"
	"fmt"
	"github.com/roboll/helmfile/helmexec"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"regexp"

	"github.com/roboll/helmfile/valuesfile"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

// HelmState structure for the helmfile
type HelmState struct {
	BaseChartPath      string
	FilePath           string
	HelmDefaults       HelmSpec         `yaml:"helmDefaults"`
	Helmfiles          []string         `yaml:"helmfiles"`
	Context            string           `yaml:"context"`
	DeprecatedReleases []ReleaseSpec    `yaml:"charts"`
	Namespace          string           `yaml:"namespace"`
	Repositories       []RepositorySpec `yaml:"repositories"`
	Releases           []ReleaseSpec    `yaml:"releases"`

	logger *zap.SugaredLogger
}

// HelmSpec to defines helmDefault values
type HelmSpec struct {
	KubeContext     string   `yaml:"kubeContext"`
	TillerNamespace string   `yaml:"tillerNamespace"`
	Args            []string `yaml:"args"`
	Verify          bool     `yaml:"verify"`
	// Wait, if set to true, will wait until all Pods, PVCs, Services, and minimum number of Pods of a Deployment are in a ready state before marking the release as successful
	Wait bool `yaml:"wait"`
	// Timeout is the time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks, and waits on pod/pvc/svc/deployment readiness) (default 300)
	Timeout int `yaml:"timeout"`
	// RecreatePods, when set to true, instruct helmfile to perform pods restart for the resource if applicable
	RecreatePods bool `yaml:"recreatePods"`
	// Force, when set to true, forces resource update through delete/recreate if needed
	Force bool `yaml:"force"`
}

// RepositorySpec that defines values for a helm repo
type RepositorySpec struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// ReleaseSpec defines the structure of a helm release
type ReleaseSpec struct {
	// Chart is the name of the chart being installed to create this release
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	Verify  *bool  `yaml:"verify"`
	// Wait, if set to true, will wait until all Pods, PVCs, Services, and minimum number of Pods of a Deployment are in a ready state before marking the release as successful
	Wait *bool `yaml:"wait"`
	// Timeout is the time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks, and waits on pod/pvc/svc/deployment readiness) (default 300)
	Timeout *int `yaml:"timeout"`
	// RecreatePods, when set to true, instruct helmfile to perform pods restart for the resource if applicable
	RecreatePods *bool `yaml:"recreatePods"`
	// Force, when set to true, forces resource update through delete/recreate if needed
	Force *bool `yaml:"force"`

	// Name is the name of this release
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
	Values    []interface{}     `yaml:"values"`
	Secrets   []string          `yaml:"secrets"`
	SetValues []SetValue        `yaml:"set"`

	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
	EnvValues []SetValue `yaml:"env"`

	// generatedValues are values that need cleaned up on exit
	generatedValues []string
}

// SetValue are the key values to set on a helm release
type SetValue struct {
	Name   string   `yaml:"name"`
	Value  string   `yaml:"value"`
	File   string   `yaml:"file"`
	Values []string `yaml:"values"`
}

func CreateFromYaml(content []byte, file string, logger *zap.SugaredLogger) (*HelmState, error) {
	var state HelmState

	state.BaseChartPath, _ = filepath.Abs(filepath.Dir(file))
	if err := yaml.UnmarshalStrict(content, &state); err != nil {
		return nil, err
	}
	state.FilePath = file

	if len(state.DeprecatedReleases) > 0 {
		if len(state.Releases) > 0 {
			return nil, fmt.Errorf("failed to parse %s: you can't specify both `charts` and `releases` sections", file)
		}
		state.Releases = state.DeprecatedReleases
		state.DeprecatedReleases = []ReleaseSpec{}
	}

	state.logger = logger

	return &state, nil
}

func (state *HelmState) applyDefaultsTo(spec *ReleaseSpec) {
	if state.Namespace != "" {
		spec.Namespace = state.Namespace
	}
}

// SyncRepos will update the given helm releases
func (state *HelmState) SyncRepos(helm helmexec.Interface) []error {
	errs := []error{}

	for _, repo := range state.Repositories {
		if err := helm.AddRepo(repo.Name, repo.URL, repo.CertFile, repo.KeyFile, repo.Username, repo.Password); err != nil {
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

type ReleaseError struct {
	*ReleaseSpec
	underlying error
}

func (e *ReleaseError) Error() string {
	return e.underlying.Error()
}

// SyncReleases wrapper for executing helm upgrade on the releases
func (state *HelmState) SyncReleases(helm helmexec.Interface, additionalValues []string, workerLimit int) []error {
	errs := []error{}
	jobQueue := make(chan *ReleaseSpec)
	doneQueue := make(chan bool)
	errQueue := make(chan *ReleaseError)

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}
	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				state.applyDefaultsTo(release)
				flags, flagsErr := state.flagsForUpgrade(helm, release)
				if flagsErr != nil {
					errQueue <- &ReleaseError{release, flagsErr}
					doneQueue <- true
					continue
				}

				haveValueErr := false
				for _, value := range additionalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errQueue <- &ReleaseError{release, err}
						haveValueErr = true
					}

					if _, err := os.Stat(valfile); os.IsNotExist(err) {
						errQueue <- &ReleaseError{release, err}
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
					errQueue <- &ReleaseError{release, err}
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

// DiffReleases wrapper for executing helm diff on the releases
func (state *HelmState) DiffReleases(helm helmexec.Interface, additionalValues []string, workerLimit int, detailedExitCode bool) []error {
	var wgRelease sync.WaitGroup
	var wgError sync.WaitGroup
	errs := []error{}
	jobQueue := make(chan *ReleaseSpec, len(state.Releases))
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}

	wgRelease.Add(len(state.Releases))

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				errs := []error{}

				state.applyDefaultsTo(release)

				flags, err := state.flagsForDiff(helm, state.BaseChartPath, release)
				if err != nil {
					errs = append(errs, err)
				}

				for _, value := range additionalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errs = append(errs, err)
					}

					if _, err := os.Stat(valfile); os.IsNotExist(err) {
						errs = append(errs, err)
					}
					flags = append(flags, "--values", valfile)
				}

				if detailedExitCode {
					flags = append(flags, "--detailed-exitcode")
				}

				if len(errs) == 0 {
					if err := helm.DiffRelease(release.Name, normalizeChart(state.BaseChartPath, release.Chart), flags...); err != nil {
						errs = append(errs, err)
					}
				}
				for _, err := range errs {
					errQueue <- err
				}
				wgRelease.Done()
			}
		}()
	}
	wgError.Add(1)
	go func() {
		for err := range errQueue {
			errs = append(errs, err)
		}
		wgError.Done()
	}()

	for i := 0; i < len(state.Releases); i++ {
		jobQueue <- &state.Releases[i]
	}

	close(jobQueue)
	wgRelease.Wait()

	close(errQueue)
	wgError.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// LintReleases wrapper for executing helm lint on the releases
func (state *HelmState) LintReleases(helm helmexec.Interface, additionalValues []string, workerLimit int) []error {
	var wgRelease sync.WaitGroup
	var wgError sync.WaitGroup
	errs := []error{}
	jobQueue := make(chan *ReleaseSpec, len(state.Releases))
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}

	wgRelease.Add(len(state.Releases))

	// Create tmp directory and bail immediately if it fails
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	defer os.RemoveAll(dir)

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				errs := []error{}
				flags, err := state.flagsForLint(helm, state.BaseChartPath, release)
				if err != nil {
					errs = append(errs, err)
				}
				for _, value := range additionalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errs = append(errs, err)
					}

					if _, err := os.Stat(valfile); os.IsNotExist(err) {
						errs = append(errs, err)
					}
					flags = append(flags, "--values", valfile)
				}

				chartPath := ""
				if pathExists(normalizeChart(state.BaseChartPath, release.Chart)) {
					chartPath = normalizeChart(state.BaseChartPath, release.Chart)
				} else {
					fetchFlags := []string{}
					if release.Version != "" {
						chartPath = path.Join(dir, release.Name, release.Version, release.Chart)
						fetchFlags = append(fetchFlags, "--version", release.Version)
					} else {
						chartPath = path.Join(dir, release.Name, "latest", release.Chart)
					}

					// only fetch chart if it is not already fetched
					if _, err := os.Stat(chartPath); os.IsNotExist(err) {
						fetchFlags = append(fetchFlags, "--untar", "--untardir", chartPath)
						if err := helm.Fetch(release.Chart, fetchFlags...); err != nil {
							errs = append(errs, err)
						}
					}
					chartPath = path.Join(chartPath, chartNameWithoutRepository(release.Chart))
				}

				if len(errs) == 0 {
					if err := helm.Lint(chartPath, flags...); err != nil {
						errs = append(errs, err)
					}
				}
				for _, err := range errs {
					errQueue <- err
				}
				wgRelease.Done()
			}
		}()
	}
	wgError.Add(1)
	go func() {
		for err := range errQueue {
			errs = append(errs, err)
		}
		wgError.Done()
	}()

	for i := 0; i < len(state.Releases); i++ {
		jobQueue <- &state.Releases[i]
	}

	close(jobQueue)
	wgRelease.Wait()

	close(errQueue)
	wgError.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (state *HelmState) ReleaseStatuses(helm helmexec.Interface, workerLimit int) []error {
	var errs []error
	jobQueue := make(chan ReleaseSpec)
	doneQueue := make(chan bool)
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}
	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				if err := helm.ReleaseStatus(release.Name); err != nil {
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

// DeleteReleases wrapper for executing helm delete on the releases
func (state *HelmState) DeleteReleases(helm helmexec.Interface, purge bool) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, release := range state.Releases {
		wg.Add(1)
		go func(wg *sync.WaitGroup, release ReleaseSpec) {
			flags := []string{}
			if purge {
				flags = append(flags, "--purge")
			}
			if err := helm.DeleteRelease(release.Name, flags...); err != nil {
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

// TestReleases wrapper for executing helm test on the releases
func (state *HelmState) TestReleases(helm helmexec.Interface, cleanup bool, timeout int) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, release := range state.Releases {
		wg.Add(1)
		go func(wg *sync.WaitGroup, release ReleaseSpec) {
			flags := []string{}
			if cleanup {
				flags = append(flags, "--cleanup")
			}
			flags = append(flags, "--timeout", strconv.Itoa(timeout))
			if err := helm.TestRelease(release.Name, flags...); err != nil {
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
	if len(filteredReleases) == 0 {
		state.logger.Debugf("specified selector did not match any releases in %s\n", state.FilePath)
		return nil
	}
	return nil
}

// UpdateDeps wrapper for updating dependencies on the releases
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

func pathExists(chart string) bool {
	_, err := os.Stat(chart)
	return err == nil
}

func chartNameWithoutRepository(chart string) string {
	chartSplit := strings.Split(chart, "/")
	return chartSplit[len(chartSplit)-1]
}

func (state *HelmState) flagsForUpgrade(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}

	if release.Verify != nil && *release.Verify || state.HelmDefaults.Verify {
		flags = append(flags, "--verify")
	}

	if release.Wait != nil && *release.Wait || state.HelmDefaults.Wait {
		flags = append(flags, "--wait")
	}

	timeout := state.HelmDefaults.Timeout
	if release.Timeout != nil {
		timeout = *release.Timeout
	}
	if timeout != 0 {
		flags = append(flags, "--timeout", fmt.Sprintf("%d", timeout))
	}

	if release.Force != nil && *release.Force || state.HelmDefaults.Force {
		flags = append(flags, "--force")
	}

	if release.RecreatePods != nil && *release.RecreatePods || state.HelmDefaults.RecreatePods {
		flags = append(flags, "--recreate-pods")
	}

	common, err := state.namespaceAndValuesFlags(helm, state.BaseChartPath, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (state *HelmState) flagsForDiff(helm helmexec.Interface, basePath string, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}
	common, err := state.namespaceAndValuesFlags(helm, basePath, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (state *HelmState) flagsForLint(helm helmexec.Interface, basePath string, release *ReleaseSpec) ([]string, error) {
	return state.namespaceAndValuesFlags(helm, basePath, release)
}

func (state *HelmState) namespaceAndValuesFlags(helm helmexec.Interface, basePath string, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Namespace != "" {
		flags = append(flags, "--namespace", release.Namespace)
	}
	for _, value := range release.Values {
		switch typedValue := value.(type) {
		case string:
			var path string
			if filepath.IsAbs(typedValue) {
				path = typedValue
			} else {
				path = filepath.Join(basePath, typedValue)
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return nil, err
			}

			valfile, err := ioutil.TempFile("", "values")
			if err != nil {
				return nil, err
			}
			defer valfile.Close()

			r := valuesfile.NewRenderer(ioutil.ReadFile, state.BaseChartPath)
			yamlBytes, err := r.RenderToBytes(path)
			if err != nil {
				return nil, err
			}

			if _, err := valfile.Write(yamlBytes); err != nil {
				return nil, fmt.Errorf("failed to write %s: %v", valfile.Name(), err)
			}
			state.logger.Debugf("successfully generated the value file at %s. produced:\n%s", path, string(yamlBytes))
			flags = append(flags, "--values", valfile.Name())

		case map[interface{}]interface{}:
			valfile, err := ioutil.TempFile("", "values")
			if err != nil {
				return nil, err
			}
			defer valfile.Close()
			encoder := yaml.NewEncoder(valfile)
			defer encoder.Close()
			if err := encoder.Encode(typedValue); err != nil {
				return nil, err
			}
			release.generatedValues = append(release.generatedValues, valfile.Name())
			flags = append(flags, "--values", valfile.Name())
		}
	}
	for _, value := range release.Secrets {
		path := filepath.Join(basePath, value)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil, err
		}

		valfile, err := helm.DecryptSecret(path)
		if err != nil {
			return nil, err
		}

		release.generatedValues = append(release.generatedValues, valfile)
		flags = append(flags, "--values", valfile)
	}
	if len(release.SetValues) > 0 {
		for _, set := range release.SetValues {
			if set.Value != "" {
				flags = append(flags, "--set", fmt.Sprintf("%s=%s", escape(set.Name), escape(set.Value)))
			} else if set.File != "" {
				flags = append(flags, "--set-file", fmt.Sprintf("%s=%s", escape(set.Name), set.File))
			} else if len(set.Values) > 0 {
				items := make([]string, len(set.Values))
				for i, raw := range set.Values {
					items[i] = escape(raw)
				}
				v := strings.Join(items, ",")
				flags = append(flags, "--set", fmt.Sprintf("%s={%s}", escape(set.Name), v))
			}
		}
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
				val = append(val, fmt.Sprintf("%s=%s", escape(set.Name), escape(value)))
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

func escape(value string) string {
	intermediate := strings.Replace(value, "{", "\\{", -1)
	intermediate = strings.Replace(intermediate, "}", "\\}", -1)
	return strings.Replace(intermediate, ",", "\\,", -1)
}
