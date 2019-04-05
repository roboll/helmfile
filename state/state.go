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

	"github.com/roboll/helmfile/helmexec"

	"regexp"

	"os/exec"
	"syscall"

	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/event"
	"github.com/roboll/helmfile/tmpl"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"net/url"
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

	removeFile func(string) error
	fileExists func(string) (bool, error)

	runner helmexec.Runner
}

// HelmSpec to defines helmDefault values
type HelmSpec struct {
	KubeContext     string   `yaml:"kubeContext"`
	TillerNamespace string   `yaml:"tillerNamespace"`
	Tillerless      bool     `yaml:"tillerless"`
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
	// Atomic, when set to true, restore previous state in case of a failed install/upgrade attempt
	Atomic bool `yaml:"atomic"`

	TLS       bool   `yaml:"tls"`
	TLSCACert string `yaml:"tlsCACert"`
	TLSKey    string `yaml:"tlsKey"`
	TLSCert   string `yaml:"tlsCert"`
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
	// Atomic, when set to true, restore previous state in case of a failed install/upgrade attempt
	Atomic *bool `yaml:"atomic"`

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

	TillerNamespace string `yaml:"tillerNamespace"`
	Tillerless      *bool  `yaml:"tillerless"`

	TLS       *bool  `yaml:"tls"`
	TLSCACert string `yaml:"tlsCACert"`
	TLSKey    string `yaml:"tlsKey"`
	TLSCert   string `yaml:"tlsCert"`

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
	releases := []*ReleaseSpec{}
	for i, _ := range st.Releases {
		if !st.Releases[i].Desired() {
			continue
		}
		releases = append(releases, &st.Releases[i])
	}

	numReleases := len(releases)
	jobs := make(chan *ReleaseSpec, numReleases)
	results := make(chan syncPrepareResult, numReleases)

	res := []syncPrepareResult{}
	errs := []error{}

	st.scatterGather(
		concurrency,
		numReleases,
		func() {
			for i := 0; i < numReleases; i++ {
				jobs <- releases[i]
			}
			close(jobs)
		},
		func(workerIndex int) {
			for release := range jobs {
				st.applyDefaultsTo(release)

				flags, flagsErr := st.flagsForUpgrade(helm, release, workerIndex)
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

					ok, err := st.fileExists(valfile)
					if err != nil {
						errs = append(errs, &ReleaseError{release, err})
					} else if !ok {
						errs = append(errs, &ReleaseError{release, fmt.Errorf("file does not exist: %s", valfile)})
					}
					flags = append(flags, "--values", valfile)
				}

				if len(errs) > 0 {
					results <- syncPrepareResult{errors: errs}
					continue
				}

				results <- syncPrepareResult{release: release, flags: flags, errors: []*ReleaseError{}}
			}
		},
		func() {
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
		},
	)

	return res, errs
}

func (st *HelmState) isReleaseInstalled(context helmexec.HelmContext, helm helmexec.Interface, release ReleaseSpec) (bool, error) {
	out, err := helm.List(context, "^"+release.Name+"$", st.tillerFlags(&release)...)
	if err != nil {
		return false, err
	} else if out != "" {
		return true, nil
	}
	return false, nil
}

func (st *HelmState) DetectReleasesToBeDeleted(helm helmexec.Interface) ([]*ReleaseSpec, error) {
	detected := []*ReleaseSpec{}
	for _, release := range st.Releases {
		if !release.Desired() {
			installed, err := st.isReleaseInstalled(st.createHelmContext(&release, 0), helm, release)
			if err != nil {
				return nil, err
			} else if installed {
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

	errs := []error{}
	jobQueue := make(chan *syncPrepareResult, len(preps))
	results := make(chan syncResult, len(preps))

	st.scatterGather(
		workerLimit,
		len(preps),
		func() {
			for i := 0; i < len(preps); i++ {
				jobQueue <- &preps[i]
			}
			close(jobQueue)
		},
		func(workerIndex int) {
			for prep := range jobQueue {
				release := prep.release
				flags := prep.flags
				chart := normalizeChart(st.basePath, release.Chart)
				var relErr *ReleaseError
				context := st.createHelmContext(release, workerIndex)
				if !release.Desired() {
					installed, err := st.isReleaseInstalled(context, helm, *release)
					if err != nil {
						relErr = &ReleaseError{release, err}
					} else if installed {
						if err := helm.DeleteRelease(context, release.Name, "--purge"); err != nil {
							relErr = &ReleaseError{release, err}
						}
					}
				} else if err := helm.SyncRelease(context, release.Name, chart, flags...); err != nil {
					relErr = &ReleaseError{release, err}
				}

				if relErr == nil {
					results <- syncResult{}
				} else {
					results <- syncResult{errors: []*ReleaseError{relErr}}
				}

				if _, err := st.triggerCleanupEvent(prep.release, "sync"); err != nil {
					st.logger.Warnf("warn: %v\n", err)
				}
			}
		},
		func() {
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
		},
	)

	if len(errs) > 0 {
		return errs
	}

	return nil
}

// downloadCharts will download and untar charts for Lint and Template
func (st *HelmState) downloadCharts(helm helmexec.Interface, dir string, concurrency int, helmfileCommand string) (map[string]string, []error) {
	temp := make(map[string]string, len(st.Releases))
	type downloadResults struct {
		releaseName string
		chartPath   string
	}
	errs := []error{}

	jobQueue := make(chan *ReleaseSpec, len(st.Releases))
	results := make(chan *downloadResults, len(st.Releases))

	st.scatterGather(
		concurrency,
		len(st.Releases),
		func() {
			for i := 0; i < len(st.Releases); i++ {
				jobQueue <- &st.Releases[i]
			}
			close(jobQueue)
		},
		func(_ int) {
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
		},
		func() {
			for i := 0; i < len(st.Releases); i++ {
				downloadRes := <-results
				temp[downloadRes.releaseName] = downloadRes.chartPath
			}
		},
	)

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
		if !release.Desired() {
			continue
		}

		flags, err := st.flagsForTemplate(helm, &release, 0)
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
		if !release.Desired() {
			continue
		}

		flags, err := st.flagsForLint(helm, &release, 0)
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
	releases := []*ReleaseSpec{}
	for i, _ := range st.Releases {
		if !st.Releases[i].Desired() {
			continue
		}
		releases = append(releases, &st.Releases[i])
	}

	numReleases := len(releases)
	jobs := make(chan *ReleaseSpec, numReleases)
	results := make(chan diffPrepareResult, numReleases)

	rs := []diffPrepareResult{}
	errs := []error{}

	st.scatterGather(
		concurrency,
		numReleases,
		func() {
			for i := 0; i < numReleases; i++ {
				jobs <- releases[i]
			}
			close(jobs)
		},
		func(workerIndex int) {
			for release := range jobs {
				errs := []error{}

				st.applyDefaultsTo(release)

				flags, err := st.flagsForDiff(helm, release, workerIndex)
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
		},
		func() {
			for i := 0; i < numReleases; i++ {
				res := <-results
				if res.errors != nil && len(res.errors) > 0 {
					for _, e := range res.errors {
						errs = append(errs, e)
					}
				} else if res.release != nil {
					rs = append(rs, res)
				}
			}
		},
	)

	return rs, errs
}

func (st *HelmState) createHelmContext(spec *ReleaseSpec, workerIndex int) helmexec.HelmContext {
	namespace := st.HelmDefaults.TillerNamespace
	if spec.TillerNamespace != "" {
		namespace = spec.TillerNamespace
	}
	tillerless := st.HelmDefaults.Tillerless
	if spec.Tillerless != nil {
		tillerless = *spec.Tillerless
	}

	return helmexec.HelmContext{
		Tillerless:      tillerless,
		TillerNamespace: namespace,
		WorkerIndex:     workerIndex,
	}
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

	rs := []*ReleaseSpec{}
	errs := []error{}

	st.scatterGather(
		workerLimit,
		len(preps),
		func() {
			for i := 0; i < len(preps); i++ {
				jobQueue <- &preps[i]
			}
			close(jobQueue)
		},
		func(workerIndex int) {
			for prep := range jobQueue {
				flags := prep.flags
				release := prep.release
				if err := helm.DiffRelease(st.createHelmContext(release, workerIndex), release.Name, normalizeChart(st.basePath, release.Chart), flags...); err != nil {
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
		},
		func() {
			for i := 0; i < len(preps); i++ {
				res := <-results
				if res.err != nil {
					errs = append(errs, res.err)
					if res.err.Code == 2 {
						rs = append(rs, res.err.ReleaseSpec)
					}
				}
			}
		},
	)

	return rs, errs
}

func (st *HelmState) ReleaseStatuses(helm helmexec.Interface, workerLimit int) []error {
	return st.scatterGatherReleases(helm, workerLimit, func(release ReleaseSpec, workerIndex int) error {
		if !release.Desired() {
			return nil
		}

		flags := []string{}
		flags = st.appendTillerFlags(flags, &release)

		return helm.ReleaseStatus(st.createHelmContext(&release, workerIndex), release.Name, flags...)
	})
}

// DeleteReleases wrapper for executing helm delete on the releases
func (st *HelmState) DeleteReleases(helm helmexec.Interface, purge bool) []error {
	return st.scatterGatherReleases(helm, len(st.Releases), func(release ReleaseSpec, workerIndex int) error {
		if !release.Desired() {
			return nil
		}

		flags := []string{}
		if purge {
			flags = append(flags, "--purge")
		}
		flags = st.appendTillerFlags(flags, &release)
		context := st.createHelmContext(&release, workerIndex)

		installed, err := st.isReleaseInstalled(context, helm, release)
		if err != nil {
			return err
		}
		if installed {
			return helm.DeleteRelease(context, release.Name, flags...)
		}
		return nil
	})
}

// TestReleases wrapper for executing helm test on the releases
func (st *HelmState) TestReleases(helm helmexec.Interface, cleanup bool, timeout int, concurrency int) []error {
	return st.scatterGatherReleases(helm, concurrency, func(release ReleaseSpec, workerIndex int) error {
		if !release.Desired() {
			return nil
		}

		flags := []string{}
		if cleanup {
			flags = append(flags, "--cleanup")
		}
		flags = append(flags, "--timeout", strconv.Itoa(timeout))
		flags = st.appendTillerFlags(flags, &release)

		return helm.TestRelease(st.createHelmContext(&release, workerIndex), release.Name, flags...)
	})
}

// Clean will remove any generated secrets
func (st *HelmState) Clean() []error {
	errs := []error{}

	for _, release := range st.Releases {
		for _, value := range release.generatedValues {
			err := st.removeFile(value)
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
		// Let the release name, namespace, and chart be used as a tag
		r.Labels["name"] = r.Name
		r.Labels["namespace"] = r.Namespace
		// Strip off just the last portion for the name stable/newrelic would give newrelic
		chartSplit := strings.Split(r.Chart, "/")
		r.Labels["chart"] = chartSplit[len(chartSplit)-1]
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

// BuildDeps wrapper for building dependencies on the releases
func (st *HelmState) BuildDeps(helm helmexec.Interface) []error {
	errs := []error{}

	for _, release := range st.Releases {
		if isLocalChart(release.Chart) {
			if err := helm.BuildDeps(normalizeChart(st.basePath, release.Chart)); err != nil {
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
	u, _ := url.Parse(path)
	if u.Scheme != "" || filepath.IsAbs(path) {
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

func (st *HelmState) appendTillerFlags(flags []string, release *ReleaseSpec) []string {
	adds := st.tillerFlags(release)
	for _, a := range adds {
		flags = append(flags, a)
	}
	return flags
}

func (st *HelmState) tillerFlags(release *ReleaseSpec) []string {
	flags := []string{}
	tillerless := st.HelmDefaults.Tillerless
	if release.Tillerless != nil {
		tillerless = *release.Tillerless
	}
	if !tillerless {
		if release.TillerNamespace != "" {
			flags = append(flags, "--tiller-namespace", release.TillerNamespace)
		} else if st.HelmDefaults.TillerNamespace != "" {
			flags = append(flags, "--tiller-namespace", st.HelmDefaults.TillerNamespace)
		}

		if release.TLS != nil && *release.TLS || release.TLS == nil && st.HelmDefaults.TLS {
			flags = append(flags, "--tls")
		}

		if release.TLSKey != "" {
			flags = append(flags, "--tls-key", release.TLSKey)
		} else if st.HelmDefaults.TLSKey != "" {
			flags = append(flags, "--tls-key", st.HelmDefaults.TLSKey)
		}

		if release.TLSCert != "" {
			flags = append(flags, "--tls-cert", release.TLSCert)
		} else if st.HelmDefaults.TLSCert != "" {
			flags = append(flags, "--tls-cert", st.HelmDefaults.TLSCert)
		}

		if release.TLSCACert != "" {
			flags = append(flags, "--tls-ca-cert", release.TLSCACert)
		} else if st.HelmDefaults.TLSCACert != "" {
			flags = append(flags, "--tls-ca-cert", st.HelmDefaults.TLSCACert)
		}
	}

	return flags
}

func (st *HelmState) flagsForUpgrade(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}

	if st.isDevelopment(release) {
		flags = append(flags, "--devel")
	}

	if release.Verify != nil && *release.Verify || release.Verify == nil && st.HelmDefaults.Verify {
		flags = append(flags, "--verify")
	}

	if release.Wait != nil && *release.Wait || release.Wait == nil && st.HelmDefaults.Wait {
		flags = append(flags, "--wait")
	}

	timeout := st.HelmDefaults.Timeout
	if release.Timeout != nil {
		timeout = *release.Timeout
	}
	if timeout != 0 {
		flags = append(flags, "--timeout", fmt.Sprintf("%d", timeout))
	}

	if release.Force != nil && *release.Force || release.Force == nil && st.HelmDefaults.Force {
		flags = append(flags, "--force")
	}

	if release.RecreatePods != nil && *release.RecreatePods || release.RecreatePods == nil && st.HelmDefaults.RecreatePods {
		flags = append(flags, "--recreate-pods")
	}

	if release.Atomic != nil && *release.Atomic || release.Atomic == nil && st.HelmDefaults.Atomic {
		flags = append(flags, "--atomic")
	}

	flags = st.appendTillerFlags(flags, release)

	common, err := st.namespaceAndValuesFlags(helm, release, workerIndex)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (st *HelmState) flagsForTemplate(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) ([]string, error) {
	flags := []string{
		"--name", release.Name,
	}
	common, err := st.namespaceAndValuesFlags(helm, release, workerIndex)
	if err != nil {
		return nil, err
	}
	return append(flags, common...), nil
}

func (st *HelmState) flagsForDiff(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) ([]string, error) {
	flags := []string{}
	if release.Version != "" {
		flags = append(flags, "--version", release.Version)
	}

	if st.isDevelopment(release) {
		flags = append(flags, "--devel")
	}

	flags = st.appendTillerFlags(flags, release)

	common, err := st.namespaceAndValuesFlags(helm, release, workerIndex)
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

func (st *HelmState) flagsForLint(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) ([]string, error) {
	return st.namespaceAndValuesFlags(helm, release, workerIndex)
}

func (st *HelmState) RenderValuesFileToBytes(path string) ([]byte, error) {
	r := tmpl.NewFileRenderer(st.readFile, filepath.Dir(path), st.envTemplateData())
	return r.RenderToBytes(path)
}

func (st *HelmState) generateTemporaryValuesFiles(values []interface{}, missingFileHandler *string) ([]string, error) {
	generatedFiles := []string{}

	for _, value := range values {
		switch typedValue := value.(type) {
		case string:
			path := st.normalizePath(typedValue)

			ok, err := st.fileExists(path)
			if err != nil {
				return nil, err
			}
			if !ok {
				if missingFileHandler == nil || *missingFileHandler == "Error" {
					return nil, fmt.Errorf("file does not exist: %s", path)
				} else if *missingFileHandler == "Warn" {
					st.logger.Warnf("skipping missing values file \"%s\"", path)
					continue
				} else if *missingFileHandler == "Info" {
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
			generatedFiles = append(generatedFiles, valfile.Name())
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
			generatedFiles = append(generatedFiles, valfile.Name())
		default:
			return nil, fmt.Errorf("unexpected type of values entry: %T", typedValue)
		}
	}
	return generatedFiles, nil
}

func (st *HelmState) namespaceAndValuesFlags(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) ([]string, error) {
	flags := []string{}
	if release.Namespace != "" {
		flags = append(flags, "--namespace", release.Namespace)
	}

	values := []interface{}{}
	for _, v := range release.Values {
		switch typedValue := v.(type) {
		case string:
			path := st.normalizePath(release.ValuesPathPrefix + typedValue)
			values = append(values, path)
		default:
			values = append(values, v)
		}
	}

	generatedFiles, err := st.generateTemporaryValuesFiles(values, release.MissingFileHandler)
	if err != nil {
		return nil, err
	}

	for _, f := range generatedFiles {
		flags = append(flags, "--values", f)
	}

	release.generatedValues = append(release.generatedValues, generatedFiles...)

	for _, value := range release.Secrets {
		path := st.normalizePath(release.ValuesPathPrefix + value)
		ok, err := st.fileExists(path)
		if err != nil {
			return nil, err
		}
		if !ok {
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

		decryptFlags := st.appendTillerFlags([]string{}, release)
		valfile, err := helm.DecryptSecret(st.createHelmContext(release, workerIndex), path, decryptFlags...)
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
