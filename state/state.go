package state

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/roboll/helmfile/helmexec"

	"regexp"

	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/valuesfile"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"os/exec"
	"syscall"
)

// HelmState structure for the helmfile
type HelmState struct {
	basePath           string
	Environments       map[string]EnvironmentSpec
	FilePath           string
	HelmDefaults       HelmSpec         `yaml:"helmDefaults"`
	Helmfiles          []string         `yaml:"helmfiles"`
	Context            string           `yaml:"context"`
	DeprecatedReleases []ReleaseSpec    `yaml:"charts"`
	Namespace          string           `yaml:"namespace"`
	Repositories       []RepositorySpec `yaml:"repositories"`
	Releases           []ReleaseSpec    `yaml:"releases"`

	env environment.Environment

	logger *zap.SugaredLogger

	readFile func(string) ([]byte, error)
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

const DefaultEnv = "default"

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

type syncResult struct {
	errors []*ReleaseError
}

type syncPrepareResult struct {
	release *ReleaseSpec
	flags   []string
	errors  []*ReleaseError
}

// SyncReleases wrapper for executing helm upgrade on the releases
func (state *HelmState) prepareSyncReleases(helm helmexec.Interface, additionalValues []string, concurrency int) ([]syncPrepareResult, []error) {
	numReleases := len(state.Releases)
	jobs := make(chan *ReleaseSpec, numReleases)
	results := make(chan syncPrepareResult, numReleases)

	if concurrency < 1 {
		concurrency = numReleases
	}
	for w := 1; w <= concurrency; w++ {
		go func() {
			for release := range jobs {
				state.applyDefaultsTo(release)
				flags, flagsErr := state.flagsForUpgrade(helm, release)
				if flagsErr != nil {
					results <- syncPrepareResult{errors: []*ReleaseError{&ReleaseError{release, flagsErr}}}
					continue
				}

				errs := []*ReleaseError{}
				for _, value := range additionalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errs = append(errs, &ReleaseError{release, err})
					}

					if _, err := os.Stat(valfile); os.IsNotExist(err) {
						errs = append(errs, &ReleaseError{release, err})
					}
					flags = append(flags, "--values", valfile)
				}

				if len(errs) > 0 {
					results <- syncPrepareResult{errors: errs}
					continue
				}

				results <- syncPrepareResult{release: release, flags: flags, errors: []*ReleaseError{}}
			}
		}()
	}

	for i := 0; i < numReleases; i++ {
		jobs <- &state.Releases[i]
	}
	close(jobs)

	res := []syncPrepareResult{}
	errs := []error{}
	for i := 0; i < numReleases; {
		select {
		case r := <-results:
			for _, e := range r.errors {
				errs = append(errs, e)
			}
			res = append(res, r)
			i++
		}
	}

	return res, errs
}

// SyncReleases wrapper for executing helm upgrade on the releases
func (state *HelmState) SyncReleases(helm helmexec.Interface, additionalValues []string, workerLimit int) []error {
	preps, prepErrs := state.prepareSyncReleases(helm, additionalValues, workerLimit)
	if len(prepErrs) > 0 {
		return prepErrs
	}

	jobQueue := make(chan *syncPrepareResult, len(preps))
	results := make(chan syncResult, len(preps))

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}
	for w := 1; w <= workerLimit; w++ {
		go func() {
			for prep := range jobQueue {
				release := prep.release
				flags := prep.flags
				chart := normalizeChart(state.basePath, release.Chart)
				if err := helm.SyncRelease(release.Name, chart, flags...); err != nil {
					results <- syncResult{errors: []*ReleaseError{&ReleaseError{release, err}}}
				} else {
					results <- syncResult{}
				}
			}
		}()
	}

	for i := 0; i < len(preps); i++ {
		jobQueue <- &preps[i]
	}
	close(jobQueue)

	errs := []error{}
	for i := 0; i < len(preps); {
		select {
		case res := <-results:
			if len(res.errors) > 0 {
				for _, e := range res.errors {
					errs = append(errs, e)
				}
			}
		}
		i++
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

// downloadCharts will download and untar charts for Lint and Template
func (state *HelmState) downloadCharts(helm helmexec.Interface, dir string, workerLimit int) (map[string]string, []error) {
	temp := make(map[string]string, len(state.Releases))
	errs := []error{}

	var wgFetch sync.WaitGroup
	jobQueue := make(chan *ReleaseSpec, len(state.Releases))
	wgFetch.Add(len(state.Releases))

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				chartPath := ""
				if pathExists(normalizeChart(state.basePath, release.Chart)) {
					chartPath = normalizeChart(state.basePath, release.Chart)
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
				temp[release.Name] = chartPath
				wgFetch.Done()
			}
		}()
	}
	for i := 0; i < len(state.Releases); i++ {
		jobQueue <- &state.Releases[i]
	}

	close(jobQueue)
	wgFetch.Wait()

	if len(errs) > 0 {
		return nil, errs
	}
	return temp, nil
}

// TemplateReleases wrapper for executing helm template on the releases
func (state *HelmState) TemplateReleases(helm helmexec.Interface, additionalValues []string, args []string, workerLimit int) []error {
	errs := []error{}
	// Create tmp directory and bail immediately if it fails
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	defer os.RemoveAll(dir)

	temp, errs := state.downloadCharts(helm, dir, workerLimit)

	if errs != nil {
		errs = append(errs, err)
		return errs
	}

	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	for _, release := range state.Releases {
		flags, err := state.flagsForTemplate(helm, &release)
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

		if len(errs) == 0 {
			if err := helm.TemplateRelease(temp[release.Name], flags...); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// LintReleases wrapper for executing helm lint on the releases
func (state *HelmState) LintReleases(helm helmexec.Interface, additionalValues []string, args []string, workerLimit int) []error {
	errs := []error{}
	// Create tmp directory and bail immediately if it fails
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	defer os.RemoveAll(dir)

	temp, errs := state.downloadCharts(helm, dir, workerLimit)
	if errs != nil {
		errs = append(errs, err)
		return errs
	}

	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	for _, release := range state.Releases {
		flags, err := state.flagsForLint(helm, &release)
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

		if len(errs) == 0 {
			if err := helm.Lint(temp[release.Name], flags...); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

type DiffError struct {
	*ReleaseSpec
	err  error
	Code int
}

func (e *DiffError) Error() string {
	return e.err.Error()
}

type diffResult struct {
	err *DiffError
}

type diffPrepareResult struct {
	release *ReleaseSpec
	flags   []string
	errors  []*ReleaseError
}

func (state *HelmState) prepareDiffReleases(helm helmexec.Interface, additionalValues []string, concurrency int, detailedExitCode, suppressSecrets bool) ([]diffPrepareResult, []error) {
	numReleases := len(state.Releases)
	jobs := make(chan *ReleaseSpec, numReleases)
	results := make(chan diffPrepareResult, numReleases)

	if concurrency < 1 {
		concurrency = numReleases
	}

	for w := 1; w <= concurrency; w++ {
		go func() {
			for release := range jobs {
				errs := []error{}

				state.applyDefaultsTo(release)

				flags, err := state.flagsForDiff(helm, release)
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

				if suppressSecrets {
					flags = append(flags, "--suppress-secrets")
				}

				if len(errs) > 0 {
					rsErrs := make([]*ReleaseError, len(errs))
					for i, e := range errs {
						rsErrs[i] = &ReleaseError{release, e}
					}
					results <- diffPrepareResult{errors: rsErrs}
				} else {
					results <- diffPrepareResult{release: release, flags: flags, errors: []*ReleaseError{}}
				}
			}
		}()
	}

	for i := 0; i < numReleases; i++ {
		jobs <- &state.Releases[i]
	}
	close(jobs)

	rs := []diffPrepareResult{}
	errs := []error{}
	for i := 0; i < numReleases; {
		select {
		case res := <-results:
			if res.errors != nil && len(res.errors) > 0 {
				for _, e := range res.errors {
					errs = append(errs, e)
				}
			} else if res.release != nil {
				rs = append(rs, res)
			}
		}
		i++
	}
	return rs, errs
}

// DiffReleases wrapper for executing helm diff on the releases
// It returns releases that had any changes
func (state *HelmState) DiffReleases(helm helmexec.Interface, additionalValues []string, workerLimit int, detailedExitCode, suppressSecrets bool) ([]*ReleaseSpec, []error) {
	preps, prepErrs := state.prepareDiffReleases(helm, additionalValues, workerLimit, detailedExitCode, suppressSecrets)
	if len(prepErrs) > 0 {
		return []*ReleaseSpec{}, prepErrs
	}

	jobQueue := make(chan *diffPrepareResult, len(preps))
	results := make(chan diffResult, len(preps))

	if workerLimit < 1 {
		workerLimit = len(state.Releases)
	}

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for prep := range jobQueue {
				flags := prep.flags
				release := prep.release
				if err := helm.DiffRelease(release.Name, normalizeChart(state.basePath, release.Chart), flags...); err != nil {
					switch e := err.(type) {
					case *exec.ExitError:
						// Propagate any non-zero exit status from the external command like `helm` that is failed under the hood
						status := e.Sys().(syscall.WaitStatus)
						results <- diffResult{&DiffError{release, err, status.ExitStatus()}}
					default:
						results <- diffResult{&DiffError{release, err, 0}}
					}
				} else {
					// diff succeeded, found no changes
					results <- diffResult{}
				}
			}
		}()
	}

	for i := 0; i < len(preps); i++ {
		jobQueue <- &preps[i]
	}
	close(jobQueue)

	rs := []*ReleaseSpec{}
	errs := []error{}
	for i := 0; i < len(preps); {
		select {
		case res := <-results:
			if res.err != nil {
				errs = append(errs, res.err)
				if res.err.Code == 2 {
					rs = append(rs, res.err.ReleaseSpec)
				}
			}
			i++
		}
	}
	close(results)
	return rs, errs
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
			if err := helm.UpdateDeps(normalizeChart(state.basePath, release.Chart)); err != nil {
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

	common, err := state.namespaceAndValuesFlags(helm, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (state *HelmState) flagsForTemplate(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	common, err := state.namespaceAndValuesFlags(helm, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (state *HelmState) flagsForDiff(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}
	common, err := state.namespaceAndValuesFlags(helm, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (state *HelmState) flagsForLint(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	return state.namespaceAndValuesFlags(helm, release)
}

func (state *HelmState) RenderValuesFileToBytes(path string) ([]byte, error) {
	r := valuesfile.NewRenderer(state.readFile, state.basePath, state.env)
	return r.RenderToBytes(path)
}

func (state *HelmState) namespaceAndValuesFlags(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
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
				path = filepath.Join(state.basePath, typedValue)
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return nil, err
			}

			yamlBytes, err := state.RenderValuesFileToBytes(path)
			if err != nil {
				return nil, fmt.Errorf("faield to render values files \"%s\": %v", typedValue, err)
			}

			valfile, err := ioutil.TempFile("", "values")
			if err != nil {
				return nil, err
			}
			defer valfile.Close()

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
		path := filepath.Join(state.basePath, value)
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
