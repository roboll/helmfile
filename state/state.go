package state

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/roboll/helmfile/helmexec"

	"regexp"

	"os/exec"
	"syscall"

	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/event"
	"github.com/roboll/helmfile/tmpl"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

// HelmState structure for the helmfile
type HelmState struct {
	basePath           string
	Environments       map[string]EnvironmentSpec
	FilePath           string
	HelmDefaults       HelmSpec         `yaml:"helmDefaults"`
	Helmfiles          []string         `yaml:"helmfiles"`
	DeprecatedContext  string           `yaml:"context"`
	DeprecatedReleases []ReleaseSpec    `yaml:"charts"`
	Namespace          string           `yaml:"namespace"`
	Repositories       []RepositorySpec `yaml:"repositories"`
	Releases           []ReleaseSpec    `yaml:"releases"`

	Templates map[string]TemplateSpec `yaml:"templates"`

	Env environment.Environment

	logger *zap.SugaredLogger

	readFile func(string) ([]byte, error)

	runner helmexec.Runner
}

// HelmSpec to defines helmDefault values
type HelmSpec struct {
	KubeContext     string   `yaml:"kubeContext"`
	TillerNamespace string   `yaml:"tillerNamespace"`
	Args            []string `yaml:"args"`
	Verify          bool     `yaml:"verify"`
	// Devel, when set to true, use development versions, too. Equivalent to version '>0.0.0-0'
	Devel bool `yaml:"devel"`
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
	// Devel, when set to true, use development versions, too. Equivalent to version '>0.0.0-0'
	Devel *bool `yaml:"devel"`
	// Wait, if set to true, will wait until all Pods, PVCs, Services, and minimum number of Pods of a Deployment are in a ready state before marking the release as successful
	Wait *bool `yaml:"wait"`
	// Timeout is the time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks, and waits on pod/pvc/svc/deployment readiness) (default 300)
	Timeout *int `yaml:"timeout"`
	// RecreatePods, when set to true, instruct helmfile to perform pods restart for the resource if applicable
	RecreatePods *bool `yaml:"recreatePods"`
	// Force, when set to true, forces resource update through delete/recreate if needed
	Force *bool `yaml:"force"`
	// Installed, when set to true, `delete --purge` the release
	Installed *bool `yaml:"installed"`

	// MissingFileHandler is set to either "Error" or "Warn". "Error" instructs helmfile to fail when unable to find a values or secrets file. When "Warn", it prints the file and continues.
	// The default value for MissingFileHandler is "Error".
	MissingFileHandler *string `yaml:"missingFileHandler"`

	// Hooks is a list of extension points paired with operations, that are executed in specific points of the lifecycle of releases defined in helmfile
	Hooks []event.Hook `yaml:"hooks"`

	// Name is the name of this release
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
	Values    []interface{}     `yaml:"values"`
	Secrets   []string          `yaml:"secrets"`
	SetValues []SetValue        `yaml:"set"`

	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
	EnvValues []SetValue `yaml:"env"`

	ValuesPathPrefix string `yaml:"valuesPathPrefix"`

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

func (st *HelmState) applyDefaultsTo(spec *ReleaseSpec) {
	if st.Namespace != "" {
		spec.Namespace = st.Namespace
	}
}

type RepoUpdater interface {
	AddRepo(name, repository, certfile, keyfile, username, password string) error
	UpdateRepo() error
}

// SyncRepos will update the given helm releases
func (st *HelmState) SyncRepos(helm RepoUpdater) []error {
	errs := []error{}

	for _, repo := range st.Repositories {
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
func (st *HelmState) prepareSyncReleases(helm helmexec.Interface, additionalValues []string, concurrency int) ([]syncPrepareResult, []error) {
	releases := st.Releases
	numReleases := len(releases)
	jobs := make(chan *ReleaseSpec, numReleases)
	results := make(chan syncPrepareResult, numReleases)

	if concurrency < 1 {
		concurrency = numReleases
	}

	// WaitGroup is required to wait until goroutine per job in job queue cleanly stops.
	var waitGroup sync.WaitGroup
	waitGroup.Add(concurrency)

	for w := 1; w <= concurrency; w++ {
		go func() {
			for release := range jobs {
				st.applyDefaultsTo(release)

				flags, flagsErr := st.flagsForUpgrade(helm, release)
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
			waitGroup.Done()
		}()
	}

	for i := 0; i < numReleases; i++ {
		jobs <- &releases[i]
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

	waitGroup.Wait()

	return res, errs
}

func (st *HelmState) DetectReleasesToBeDeleted(helm helmexec.Interface) ([]*ReleaseSpec, error) {
	detected := []*ReleaseSpec{}
	for i, _ := range st.Releases {
		release := st.Releases[i]
		if release.Installed != nil && !*release.Installed {
			err := helm.ReleaseStatus(release.Name)
			if err != nil {
				switch e := err.(type) {
				case *exec.ExitError:
					// Propagate any non-zero exit status from the external command like `helm` that is failed under the hood
					status := e.Sys().(syscall.WaitStatus)
					if status.ExitStatus() != 1 {
						return nil, e
					}
				default:
					return nil, e
				}
			} else {
				detected = append(detected, &release)
			}
		}
	}
	return detected, nil
}

// SyncReleases wrapper for executing helm upgrade on the releases
func (st *HelmState) SyncReleases(helm helmexec.Interface, additionalValues []string, workerLimit int) []error {
	preps, prepErrs := st.prepareSyncReleases(helm, additionalValues, workerLimit)
	if len(prepErrs) > 0 {
		return prepErrs
	}

	jobQueue := make(chan *syncPrepareResult, len(preps))
	results := make(chan syncResult, len(preps))

	if workerLimit < 1 {
		workerLimit = len(preps)
	}

	// WaitGroup is required to wait until goroutine per job in job queue cleanly stops.
	// Otherwise, cleanup hooks won't run fully.
	// See #363 for more context.
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerLimit)

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for prep := range jobQueue {
				release := prep.release
				flags := prep.flags
				chart := normalizeChart(st.basePath, release.Chart)
				if release.Installed != nil && !*release.Installed {
					if err := helm.ReleaseStatus(release.Name); err == nil {
						if err := helm.DeleteRelease(release.Name, "--purge"); err != nil {
							results <- syncResult{errors: []*ReleaseError{&ReleaseError{release, err}}}
						} else {
							results <- syncResult{}
						}
					}
				} else if err := helm.SyncRelease(release.Name, chart, flags...); err != nil {
					results <- syncResult{errors: []*ReleaseError{&ReleaseError{release, err}}}
				} else {
					results <- syncResult{}
				}

				if _, err := st.triggerCleanupEvent(prep.release, "sync"); err != nil {
					st.logger.Warnf("warn: %v\n", err)
				}
			}
			waitGroup.Done()
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

	waitGroup.Wait()

	if len(errs) > 0 {
		return errs
	}

	return nil
}

// downloadCharts will download and untar charts for Lint and Template
func (st *HelmState) downloadCharts(helm helmexec.Interface, dir string, workerLimit int, helmfileCommand string) (map[string]string, []error) {
	temp := make(map[string]string, len(st.Releases))
	type downloadResults struct {
		releaseName string
		chartPath   string
	}
	errs := []error{}

	var wgFetch sync.WaitGroup
	jobQueue := make(chan *ReleaseSpec, len(st.Releases))
	results := make(chan *downloadResults, len(st.Releases))
	wgFetch.Add(len(st.Releases))

	if workerLimit < 1 {
		workerLimit = len(st.Releases)
	}

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				chartPath := ""
				if pathExists(normalizeChart(st.basePath, release.Chart)) {
					chartPath = normalizeChart(st.basePath, release.Chart)
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
					// Set chartPath to be the path containing Chart.yaml, if found
					fullChartPath, err := findChartDirectory(chartPath)
					if err == nil {
						chartPath = filepath.Dir(fullChartPath)
					}
				}

				results <- &downloadResults{release.Name, chartPath}
			}
			wgFetch.Done()
		}()
	}
	for i := 0; i < len(st.Releases); i++ {
		jobQueue <- &st.Releases[i]
	}
	close(jobQueue)

	for i := 0; i < len(st.Releases); i++ {
		downloadRes := <-results
		temp[downloadRes.releaseName] = downloadRes.chartPath
	}

	wgFetch.Wait()

	if len(errs) > 0 {
		return nil, errs
	}
	return temp, nil
}

// TemplateReleases wrapper for executing helm template on the releases
func (st *HelmState) TemplateReleases(helm helmexec.Interface, additionalValues []string, args []string, workerLimit int) []error {
	errs := []error{}
	// Create tmp directory and bail immediately if it fails
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	defer os.RemoveAll(dir)

	temp, errs := st.downloadCharts(helm, dir, workerLimit, "template")

	if errs != nil {
		errs = append(errs, err)
		return errs
	}

	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	for _, release := range st.Releases {
		flags, err := st.flagsForTemplate(helm, &release)
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

		if _, err := st.triggerCleanupEvent(&release, "template"); err != nil {
			st.logger.Warnf("warn: %v\n", err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// LintReleases wrapper for executing helm lint on the releases
func (st *HelmState) LintReleases(helm helmexec.Interface, additionalValues []string, args []string, workerLimit int) []error {
	errs := []error{}
	// Create tmp directory and bail immediately if it fails
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		errs = append(errs, err)
		return errs
	}
	defer os.RemoveAll(dir)

	temp, errs := st.downloadCharts(helm, dir, workerLimit, "lint")
	if errs != nil {
		errs = append(errs, err)
		return errs
	}

	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	for _, release := range st.Releases {
		flags, err := st.flagsForLint(helm, &release)
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

		if _, err := st.triggerCleanupEvent(&release, "lint"); err != nil {
			st.logger.Warnf("warn: %v\n", err)
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

func (st *HelmState) prepareDiffReleases(helm helmexec.Interface, additionalValues []string, concurrency int, detailedExitCode, suppressSecrets bool) ([]diffPrepareResult, []error) {
	releases := []ReleaseSpec{}
	for _, r := range st.Releases {
		if r.Installed == nil || *r.Installed {
			releases = append(releases, r)
		}
	}
	numReleases := len(releases)
	jobs := make(chan *ReleaseSpec, numReleases)
	results := make(chan diffPrepareResult, numReleases)

	if concurrency < 1 {
		concurrency = numReleases
	}

	// WaitGroup is required to wait until goroutine per job in job queue cleanly stops.
	var waitGroup sync.WaitGroup
	waitGroup.Add(concurrency)

	for w := 1; w <= concurrency; w++ {
		go func() {
			for release := range jobs {
				errs := []error{}

				st.applyDefaultsTo(release)

				flags, err := st.flagsForDiff(helm, release)
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
			waitGroup.Done()
		}()
	}

	for i := 0; i < numReleases; i++ {
		jobs <- &releases[i]
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

	waitGroup.Wait()

	return rs, errs
}

// DiffReleases wrapper for executing helm diff on the releases
// It returns releases that had any changes
func (st *HelmState) DiffReleases(helm helmexec.Interface, additionalValues []string, workerLimit int, detailedExitCode, suppressSecrets bool, triggerCleanupEvents bool) ([]*ReleaseSpec, []error) {
	preps, prepErrs := st.prepareDiffReleases(helm, additionalValues, workerLimit, detailedExitCode, suppressSecrets)
	if len(prepErrs) > 0 {
		return []*ReleaseSpec{}, prepErrs
	}

	jobQueue := make(chan *diffPrepareResult, len(preps))
	results := make(chan diffResult, len(preps))

	if workerLimit < 1 {
		workerLimit = len(preps)
	}

	// WaitGroup is required to wait until goroutine per job in job queue cleanly stops.
	// Otherwise, cleanup hooks won't run fully.
	// See #363 for more context.
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerLimit)

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for prep := range jobQueue {
				flags := prep.flags
				release := prep.release
				if err := helm.DiffRelease(release.Name, normalizeChart(st.basePath, release.Chart), flags...); err != nil {
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

				if triggerCleanupEvents {
					if _, err := st.triggerCleanupEvent(prep.release, "diff"); err != nil {
						st.logger.Warnf("warn: %v\n", err)
					}
				}
			}
			waitGroup.Done()
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

	waitGroup.Wait()

	return rs, errs
}

func (st *HelmState) ReleaseStatuses(helm helmexec.Interface, workerLimit int) []error {
	var errs []error
	jobQueue := make(chan ReleaseSpec)
	doneQueue := make(chan bool)
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(st.Releases)
	}

	// WaitGroup is required to wait until goroutine per job in job queue cleanly stops.
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerLimit)

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for release := range jobQueue {
				if err := helm.ReleaseStatus(release.Name); err != nil {
					errQueue <- err
				}
				doneQueue <- true
			}
			waitGroup.Done()
		}()
	}

	go func() {
		for _, release := range st.Releases {
			jobQueue <- release
		}
		close(jobQueue)
	}()

	for i := 0; i < len(st.Releases); {
		select {
		case err := <-errQueue:
			errs = append(errs, err)
		case <-doneQueue:
			i++
		}
	}

	waitGroup.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// DeleteReleases wrapper for executing helm delete on the releases
func (st *HelmState) DeleteReleases(helm helmexec.Interface, purge bool) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, release := range st.Releases {
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
func (st *HelmState) TestReleases(helm helmexec.Interface, cleanup bool, timeout int) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, release := range st.Releases {
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
func (st *HelmState) Clean() []error {
	errs := []error{}

	for _, release := range st.Releases {
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
func (st *HelmState) FilterReleases(labels []string) error {
	var filteredReleases []ReleaseSpec
	releaseSet := map[string][]ReleaseSpec{}
	filters := []ReleaseFilter{}
	for _, label := range labels {
		f, err := ParseLabels(label)
		if err != nil {
			return err
		}
		filters = append(filters, f)
	}
	for _, r := range st.Releases {
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
				releaseSet[r.Name] = append(releaseSet[r.Name], r)
				continue
			}
		}
	}
	for _, r := range releaseSet {
		filteredReleases = append(filteredReleases, r...)
	}
	st.Releases = filteredReleases
	numFound := len(filteredReleases)
	st.logger.Debugf("%d release(s) matching %s found in %s\n", numFound, strings.Join(labels, ","), st.FilePath)
	return nil
}

func (st *HelmState) PrepareRelease(helm helmexec.Interface, helmfileCommand string) []error {
	errs := []error{}

	for _, release := range st.Releases {
		if _, err := st.triggerPrepareEvent(&release, helmfileCommand); err != nil {
			errs = append(errs, &ReleaseError{&release, err})
			continue
		}
	}
	if len(errs) != 0 {
		return errs
	}
	return nil
}

func (st *HelmState) triggerPrepareEvent(r *ReleaseSpec, helmfileCommand string) (bool, error) {
	return st.triggerReleaseEvent("prepare", r, helmfileCommand)
}

func (st *HelmState) triggerCleanupEvent(r *ReleaseSpec, helmfileCommand string) (bool, error) {
	return st.triggerReleaseEvent("cleanup", r, helmfileCommand)
}

func (st *HelmState) triggerReleaseEvent(evt string, r *ReleaseSpec, helmfileCmd string) (bool, error) {
	bus := &event.Bus{
		Hooks:         r.Hooks,
		StateFilePath: st.FilePath,
		BasePath:      st.basePath,
		Namespace:     st.Namespace,
		Env:           st.Env,
		Logger:        st.logger,
		ReadFile:      st.readFile,
	}
	data := map[string]interface{}{
		"Release":         r,
		"HelmfileCommand": helmfileCmd,
	}
	return bus.Trigger(evt, data)
}

// UpdateDeps wrapper for updating dependencies on the releases
func (st *HelmState) UpdateDeps(helm helmexec.Interface) []error {
	errs := []error{}

	for _, release := range st.Releases {
		if isLocalChart(release.Chart) {
			if err := helm.UpdateDeps(normalizeChart(st.basePath, release.Chart)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) != 0 {
		return errs
	}
	return nil
}

// JoinBase returns an absolute path in the form basePath/relative
func (st *HelmState) JoinBase(relPath string) string {
	return filepath.Join(st.basePath, relPath)
}

// normalizes relative path to absolute one
func (st *HelmState) normalizePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	} else {
		return st.JoinBase(path)
	}
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

// find "Chart.yaml"
func findChartDirectory(topLevelDir string) (string, error) {
	var files []string
	filepath.Walk(topLevelDir, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			r, err := regexp.MatchString("Chart.yaml", f.Name())
			if err == nil && r {
				files = append(files, path)
			}
		}
		return nil
	})
	// Sort to get the shortest path
	sort.Strings(files)
	if len(files) > 0 {
		first := files[0]
		return first, nil
	}

	return topLevelDir, errors.New("No Chart.yaml found")
}

func (st *HelmState) flagsForUpgrade(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}

	if st.isDevelopment(release) {
		flags = append(flags, "--devel")
	}

	if release.Verify != nil && *release.Verify || st.HelmDefaults.Verify {
		flags = append(flags, "--verify")
	}

	if release.Wait != nil && *release.Wait || st.HelmDefaults.Wait {
		flags = append(flags, "--wait")
	}

	timeout := st.HelmDefaults.Timeout
	if release.Timeout != nil {
		timeout = *release.Timeout
	}
	if timeout != 0 {
		flags = append(flags, "--timeout", fmt.Sprintf("%d", timeout))
	}

	if release.Force != nil && *release.Force || st.HelmDefaults.Force {
		flags = append(flags, "--force")
	}

	if release.RecreatePods != nil && *release.RecreatePods || st.HelmDefaults.RecreatePods {
		flags = append(flags, "--recreate-pods")
	}

	common, err := st.namespaceAndValuesFlags(helm, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (st *HelmState) flagsForTemplate(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{
		"--name", release.Name,
	}
	common, err := st.namespaceAndValuesFlags(helm, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (st *HelmState) flagsForDiff(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}

	if st.isDevelopment(release) {
		flags = append(flags, "--devel")
	}

	common, err := st.namespaceAndValuesFlags(helm, release)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (st *HelmState) isDevelopment(release *ReleaseSpec) bool {
	result := st.HelmDefaults.Devel
	if release.Devel != nil {
		result = *release.Devel
	}

	return result
}

func (st *HelmState) flagsForLint(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	return st.namespaceAndValuesFlags(helm, release)
}

func (st *HelmState) RenderValuesFileToBytes(path string) ([]byte, error) {
	r := tmpl.NewFileRenderer(st.readFile, filepath.Dir(path), st.envTemplateData())
	return r.RenderToBytes(path)
}

func (st *HelmState) namespaceAndValuesFlags(helm helmexec.Interface, release *ReleaseSpec) ([]string, error) {
	flags := []string{}
	if release.Namespace != "" {
		flags = append(flags, "--namespace", release.Namespace)
	}
	for _, value := range release.Values {
		switch typedValue := value.(type) {
		case string:
			path := st.normalizePath(release.ValuesPathPrefix + typedValue)

			if _, err := os.Stat(path); os.IsNotExist(err) {
				if release.MissingFileHandler == nil || *release.MissingFileHandler == "Error" {
					return nil, err
				} else if *release.MissingFileHandler == "Warn" {
					st.logger.Warnf("skipping missing values file \"%s\"", path)
					continue
				} else if *release.MissingFileHandler == "Info" {
					st.logger.Infof("skipping missing values file \"%s\"", path)
					continue
				} else {
					st.logger.Debugf("skipping missing values file \"%s\"", path)
					continue
				}
			}

			yamlBytes, err := st.RenderValuesFileToBytes(path)
			if err != nil {
				return nil, fmt.Errorf("failed to render values files \"%s\": %v", typedValue, err)
			}

			valfile, err := ioutil.TempFile("", "values")
			if err != nil {
				return nil, err
			}
			defer valfile.Close()

			if _, err := valfile.Write(yamlBytes); err != nil {
				return nil, fmt.Errorf("failed to write %s: %v", valfile.Name(), err)
			}
			st.logger.Debugf("successfully generated the value file at %s. produced:\n%s", path, string(yamlBytes))
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
		path := st.normalizePath(release.ValuesPathPrefix + value)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if release.MissingFileHandler == nil || *release.MissingFileHandler == "Error" {
				return nil, err
			} else if *release.MissingFileHandler == "Warn" {
				st.logger.Warnf("skipping missing secrets file \"%s\"", path)
				continue
			} else if *release.MissingFileHandler == "Info" {
				st.logger.Infof("skipping missing secrets file \"%s\"", path)
				continue
			} else {
				st.logger.Debugf("skipping missing secrets file \"%s\"", path)
				continue
			}
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
				flags = append(flags, "--set-file", fmt.Sprintf("%s=%s", escape(set.Name), st.normalizePath(set.File)))
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
