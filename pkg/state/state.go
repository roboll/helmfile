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

	"github.com/roboll/helmfile/pkg/environment"
	"github.com/roboll/helmfile/pkg/event"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/remote"
	"github.com/roboll/helmfile/pkg/tmpl"

	"regexp"

	"github.com/tatsushid/go-prettytable"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

// HelmState structure for the helmfile
type HelmState struct {
	basePath string
	FilePath string

	// DefaultValues is the default values to be overrode by environment values and command-line overrides
	DefaultValues []interface{} `yaml:"values"`

	Environments map[string]EnvironmentSpec `yaml:"environments"`

	Bases              []string          `yaml:"bases"`
	HelmDefaults       HelmSpec          `yaml:"helmDefaults"`
	Helmfiles          []SubHelmfileSpec `yaml:"helmfiles"`
	DeprecatedContext  string            `yaml:"context"`
	DeprecatedReleases []ReleaseSpec     `yaml:"charts"`
	Namespace          string            `yaml:"namespace"`
	Repositories       []RepositorySpec  `yaml:"repositories"`
	Releases           []ReleaseSpec     `yaml:"releases"`
	Selectors          []string

	Templates map[string]TemplateSpec `yaml:"templates"`

	Env environment.Environment

	logger *zap.SugaredLogger

	readFile func(string) ([]byte, error)

	removeFile func(string) error
	fileExists func(string) (bool, error)
	glob       func(string) ([]string, error)
	tempDir    func(string, string) (string, error)

	runner helmexec.Runner

	vals      map[string]interface{}
	valsMutex sync.Mutex
}

// SubHelmfileSpec defines the subhelmfile path and options
type SubHelmfileSpec struct {
	Path               string   //path or glob pattern for the sub helmfiles
	Selectors          []string //chosen selectors for the sub helmfiles
	SelectorsInherited bool     //do the sub helmfiles inherits from parent selectors

	Environment SubhelmfileEnvironmentSpec
}

type SubhelmfileEnvironmentSpec struct {
	OverrideValues []interface{} `yaml:"values"`
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

	KubeContext string `yaml:"kubeContext"`

	TLS       *bool  `yaml:"tls"`
	TLSCACert string `yaml:"tlsCACert"`
	TLSKey    string `yaml:"tlsKey"`
	TLSCert   string `yaml:"tlsCert"`

	// These settings requires helm-x integration to work
	Dependencies          []Dependency  `yaml:"dependencies"`
	JSONPatches           []interface{} `yaml:"jsonPatches"`
	StrategicMergePatches []interface{} `yaml:"strategicMergePatches"`

	// generatedValues are values that need cleaned up on exit
	generatedValues []string
	//version of the chart that has really been installed cause desired version may be fuzzy (~2.0.0)
	installedVersion string
}

// SetValue are the key values to set on a helm release
type SetValue struct {
	Name   string   `yaml:"name"`
	Value  string   `yaml:"value"`
	File   string   `yaml:"file"`
	Values []string `yaml:"values"`
}

// AffectedReleases hold the list of released that where updated, deleted, or in error
type AffectedReleases struct {
	Upgraded []*ReleaseSpec
	Deleted  []*ReleaseSpec
	Failed   []*ReleaseSpec
}

const DefaultEnv = "default"

const MissingFileHandlerError = "Error"
const MissingFileHandlerInfo = "Info"
const MissingFileHandlerWarn = "Warn"
const MissingFileHandlerDebug = "Debug"

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

				// If `installed: false`, the only potential operation on this release would be uninstalling.
				// We skip generating values files in that case, because for an uninstall with `helm delete`, we don't need to those.
				// The values files are for `helm upgrade -f values.yaml` calls that happens when the release has `installed: true`.
				// This logic addresses:
				// - https://github.com/roboll/helmfile/issues/519
				// - https://github.com/roboll/helmfile/issues/616
				if !release.Desired() {
					results <- syncPrepareResult{release: release, flags: []string{}, errors: []*ReleaseError{}}
					continue
				}

				flags, flagsErr := st.flagsForUpgrade(helm, release, workerIndex)
				if flagsErr != nil {
					results <- syncPrepareResult{errors: []*ReleaseError{newReleaseError(release, flagsErr)}}
					continue
				}

				errs := []*ReleaseError{}
				for _, value := range additionalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errs = append(errs, newReleaseError(release, err))
					}

					ok, err := st.fileExists(valfile)
					if err != nil {
						errs = append(errs, newReleaseError(release, err))
					} else if !ok {
						errs = append(errs, newReleaseError(release, fmt.Errorf("file does not exist: %s", valfile)))
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
	out, err := helm.List(context, "^"+release.Name+"$", st.connectionFlags(&release)...)
	if err != nil {
		return false, err
	} else if out != "" {
		return true, nil
	}
	return false, nil
}

func (st *HelmState) DetectReleasesToBeDeleted(helm helmexec.Interface) ([]*ReleaseSpec, error) {
	detected := []*ReleaseSpec{}
	for i := range st.Releases {
		release := st.Releases[i]

		if !release.Desired() {
			installed, err := st.isReleaseInstalled(st.createHelmContext(&release, 0), helm, release)
			if err != nil {
				return nil, err
			} else if installed {
				// Otherwise `release` messed up(https://github.com/roboll/helmfile/issues/554)
				r := release
				detected = append(detected, &r)
			}
		}
	}
	return detected, nil
}

// SyncReleases wrapper for executing helm upgrade on the releases
func (st *HelmState) SyncReleases(affectedReleases *AffectedReleases, helm helmexec.Interface, additionalValues []string, workerLimit int) []error {
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

				if _, err := st.triggerPresyncEvent(release, "sync"); err != nil {
					relErr = newReleaseError(release, err)
				} else if !release.Desired() {
					installed, err := st.isReleaseInstalled(context, helm, *release)
					if err != nil {
						relErr = newReleaseError(release, err)
					} else if installed {
						deletionFlags := st.appendConnectionFlags([]string{"--purge"}, release)
						if err := helm.DeleteRelease(context, release.Name, deletionFlags...); err != nil {
							affectedReleases.Failed = append(affectedReleases.Failed, release)
							relErr = newReleaseError(release, err)
						} else {
							affectedReleases.Deleted = append(affectedReleases.Deleted, release)
						}
					}
				} else if err := helm.SyncRelease(context, release.Name, chart, flags...); err != nil {
					affectedReleases.Failed = append(affectedReleases.Failed, release)
					relErr = newReleaseError(release, err)
				} else {
					affectedReleases.Upgraded = append(affectedReleases.Upgraded, release)
					installedVersion, err := st.getDeployedVersion(context, helm, release)
					if err != nil { //err is not really impacting so just log it
						st.logger.Debugf("getting deployed release version failed:%v", err)
					} else {
						release.installedVersion = installedVersion
					}
				}

				if relErr == nil {
					results <- syncResult{}
				} else {
					results <- syncResult{errors: []*ReleaseError{relErr}}
				}

				if _, err := st.triggerPostsyncEvent(release, "sync"); err != nil {
					st.logger.Warnf("warn: %v\n", err)
				}

				if _, err := st.triggerCleanupEvent(release, "sync"); err != nil {
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

func (st *HelmState) getDeployedVersion(context helmexec.HelmContext, helm helmexec.Interface, release *ReleaseSpec) (string, error) {
	//retrieve the version
	if out, err := helm.List(context, "^"+release.Name+"$", st.connectionFlags(release)...); err == nil {
		chartName := filepath.Base(release.Chart)
		//the regexp without escapes : .*\s.*\s.*\s.*\schartName-(.*?)\s
		pat := regexp.MustCompile(".*\\s.*\\s.*\\s.*\\s" + chartName + "-(.*?)\\s")
		versions := pat.FindStringSubmatch(out)
		if len(versions) > 0 {
			return versions[1], nil
		} else {
			//fails to find the version
			return "failed to get version", errors.New("Failed to get the version for:" + chartName)
		}
	} else {
		return "failed to get version", err
	}
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

					if st.isDevelopment(release) {
						fetchFlags = append(fetchFlags, "--devel")
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
	// Reset the extra args if already set, not to break `helm fetch` by adding the args intended for `lint`
	helm.SetExtraArgs()

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

	for i := range st.Releases {
		release := st.Releases[i]

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
	// Reset the extra args if already set, not to break `helm fetch` by adding the args intended for `lint`
	helm.SetExtraArgs()

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

	for i := range st.Releases {
		release := st.Releases[i]

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

type diffResult struct {
	err *ReleaseError
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
						rsErrs[i] = newReleaseError(release, e)
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
					case helmexec.ExitError:
						// Propagate any non-zero exit status from the external command like `helm` that is failed under the hood
						results <- diffResult{&ReleaseError{release, err, e.ExitStatus()}}
					default:
						results <- diffResult{&ReleaseError{release, err, 0}}
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
		flags = st.appendConnectionFlags(flags, &release)

		return helm.ReleaseStatus(st.createHelmContext(&release, workerIndex), release.Name, flags...)
	})
}

// DeleteReleases wrapper for executing helm delete on the releases
func (st *HelmState) DeleteReleases(affectedReleases *AffectedReleases, helm helmexec.Interface, concurrency int, purge bool) []error {
	return st.scatterGatherReleases(helm, concurrency, func(release ReleaseSpec, workerIndex int) error {
		if !release.Desired() {
			return nil
		}

		flags := []string{}
		if purge {
			flags = append(flags, "--purge")
		}
		flags = st.appendConnectionFlags(flags, &release)
		context := st.createHelmContext(&release, workerIndex)

		installed, err := st.isReleaseInstalled(context, helm, release)
		if err != nil {
			return err
		}
		if installed {
			if err := helm.DeleteRelease(context, release.Name, flags...); err != nil {
				affectedReleases.Failed = append(affectedReleases.Failed, &release)
				return err
			} else {
				affectedReleases.Deleted = append(affectedReleases.Deleted, &release)
				return nil
			}
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
		flags = st.appendConnectionFlags(flags, &release)

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
func (st *HelmState) FilterReleases() error {
	var filteredReleases []ReleaseSpec
	releaseSet := map[string][]ReleaseSpec{}
	filters := []ReleaseFilter{}
	for _, label := range st.Selectors {
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
	st.logger.Debugf("%d release(s) matching %s found in %s\n", numFound, strings.Join(st.Selectors, ","), st.FilePath)
	return nil
}

func (st *HelmState) PrepareReleases(helm helmexec.Interface, helmfileCommand string) []error {
	errs := []error{}

	for i := range st.Releases {
		release := st.Releases[i]

		if _, err := st.triggerPrepareEvent(&release, helmfileCommand); err != nil {
			errs = append(errs, newReleaseError(&release, err))
			continue
		}
	}
	if len(errs) != 0 {
		return errs
	}

	updated, err := st.ResolveDeps()
	if err != nil {
		return []error{err}
	}

	*st = *updated

	return nil
}

func (st *HelmState) triggerPrepareEvent(r *ReleaseSpec, helmfileCommand string) (bool, error) {
	return st.triggerReleaseEvent("prepare", r, helmfileCommand)
}

func (st *HelmState) triggerCleanupEvent(r *ReleaseSpec, helmfileCommand string) (bool, error) {
	return st.triggerReleaseEvent("cleanup", r, helmfileCommand)
}

func (st *HelmState) triggerPresyncEvent(r *ReleaseSpec, helmfileCommand string) (bool, error) {
	return st.triggerReleaseEvent("presync", r, helmfileCommand)
}

func (st *HelmState) triggerPostsyncEvent(r *ReleaseSpec, helmfileCommand string) (bool, error) {
	return st.triggerReleaseEvent("postsync", r, helmfileCommand)
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

// ResolveDeps returns a copy of this helmfile state with the concrete chart version numbers filled in for remote chart dependencies
func (st *HelmState) ResolveDeps() (*HelmState, error) {
	return st.mergeLockedDependencies()
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

	if len(errs) == 0 {
		tempDir := st.tempDir
		if tempDir == nil {
			tempDir = ioutil.TempDir
		}
		_, err := st.updateDependenciesInTempDir(helm, tempDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to update deps: %v", err))
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
	filepath.Walk(topLevelDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking through %s: %v", path, err)
		}
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

// appendConnectionFlags append all the helm command-line flags related to K8s API and Tiller connection including the kubecontext
func (st *HelmState) appendConnectionFlags(flags []string, release *ReleaseSpec) []string {
	adds := st.connectionFlags(release)
	for _, a := range adds {
		flags = append(flags, a)
	}
	return flags
}

func (st *HelmState) connectionFlags(release *ReleaseSpec) []string {
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

		if release.KubeContext != "" {
			flags = append(flags, "--kube-context", release.KubeContext)
		} else if st.HelmDefaults.KubeContext != "" {
			flags = append(flags, "--kube-context", st.HelmDefaults.KubeContext)
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

	flags = st.appendConnectionFlags(flags, release)

	var err error
	flags, err = st.appendHelmXFlags(flags, release)
	if err != nil {
		return nil, err
	}

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

	var err error
	flags, err = st.appendHelmXFlags(flags, release)
	if err != nil {
		return nil, err
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

	flags = st.appendConnectionFlags(flags, release)

	var err error
	flags, err = st.appendHelmXFlags(flags, release)
	if err != nil {
		return nil, err
	}

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
	flags, err := st.namespaceAndValuesFlags(helm, release, workerIndex)
	if err != nil {
		return nil, err
	}

	flags, err = st.appendHelmXFlags(flags, release)
	if err != nil {
		return nil, err
	}

	return flags, nil
}

func (st *HelmState) RenderValuesFileToBytes(path string) ([]byte, error) {
	r := tmpl.NewFileRenderer(st.readFile, filepath.Dir(path), st.valuesFileTemplateData())
	return r.RenderToBytes(path)
}

func (st *HelmState) storage() *Storage {
	return &Storage{
		FilePath: st.FilePath,
		basePath: st.basePath,
		glob:     st.glob,
		logger:   st.logger,
	}
}

func (st *HelmState) ExpandedHelmfiles() ([]SubHelmfileSpec, error) {
	helmfiles := []SubHelmfileSpec{}
	for _, hf := range st.Helmfiles {
		if remote.IsRemote(hf.Path) {
			helmfiles = append(helmfiles, hf)
			continue
		}

		matches, err := st.storage().ExpandPaths(hf.Path)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no file matching %s found", hf.Path)
		}
		for _, match := range matches {
			newHelmfile := hf
			newHelmfile.Path = match
			helmfiles = append(helmfiles, newHelmfile)
		}
	}

	return helmfiles, nil
}

func (st *HelmState) generateTemporaryValuesFiles(values []interface{}, missingFileHandler *string) ([]string, error) {
	generatedFiles := []string{}

	for _, value := range values {
		switch typedValue := value.(type) {
		case string:
			paths, skip, err := st.storage().resolveFile(missingFileHandler, "values", typedValue)
			if err != nil {
				return nil, err
			}
			if skip {
				continue
			}

			if len(paths) > 1 {
				return nil, fmt.Errorf("glob patterns in release values and secrets is not supported yet. please submit a feature request if necessary")
			}
			path := paths[0]

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
			return nil, fmt.Errorf("unexpected type of value: value=%v, type=%T", typedValue, typedValue)
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
			path := st.storage().normalizePath(release.ValuesPathPrefix + typedValue)
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
		paths, skip, err := st.storage().resolveFile(release.MissingFileHandler, "secrets", release.ValuesPathPrefix+value)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}

		if len(paths) > 1 {
			return nil, fmt.Errorf("glob patterns in release secret file is not supported yet. please submit a feature request if necessary")
		}
		path := paths[0]

		decryptFlags := st.appendConnectionFlags([]string{}, release)
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
				flags = append(flags, "--set-file", fmt.Sprintf("%s=%s", escape(set.Name), st.storage().normalizePath(set.File)))
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

// DisplayAffectedReleases logs the upgraded, deleted and in error releases
func (ar *AffectedReleases) DisplayAffectedReleases(logger *zap.SugaredLogger) {
	if ar.Upgraded != nil {
		logger.Info("\nList of updated releases :")
		tbl, _ := prettytable.NewTable(prettytable.Column{Header: "RELEASE"},
			prettytable.Column{Header: "CHART", MinWidth: 6},
			prettytable.Column{Header: "VERSION", AlignRight: true},
		)
		tbl.Separator = "   "
		for _, release := range ar.Upgraded {
			tbl.AddRow(release.Name, release.Chart, release.installedVersion)
		}
		tbl.Print()
	}
	if ar.Deleted != nil {
		logger.Info("\nList of deleted releases :")
		logger.Info("RELEASE")
		for _, release := range ar.Deleted {
			logger.Info(release.Name)
		}
	}
	if ar.Failed != nil {
		logger.Info("\nList of releases in error :")
		logger.Info("RELEASE")
		for _, release := range ar.Failed {
			logger.Info(release.Name)
		}
	}
}

func escape(value string) string {
	intermediate := strings.Replace(value, "{", "\\{", -1)
	intermediate = strings.Replace(intermediate, "}", "\\}", -1)
	return strings.Replace(intermediate, ",", "\\,", -1)
}

//UnmarshalYAML will unmarshal the helmfile yaml section and fill the SubHelmfileSpec structure
//this is required to keep allowing string scalar for defining helmfile
func (hf *SubHelmfileSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var tmp interface{}
	if err := unmarshal(&tmp); err != nil {
		return err
	}

	switch i := tmp.(type) {
	case string: // single path definition without sub items, legacy sub helmfile definition
		hf.Path = i
	case map[interface{}]interface{}: // helmfile path with sub section
		var subHelmfileSpecTmp struct {
			Path               string   `yaml:"path"`
			Selectors          []string `yaml:"selectors"`
			SelectorsInherited bool     `yaml:"selectorsInherited"`

			Environment SubhelmfileEnvironmentSpec `yaml:",inline"`
		}
		if err := unmarshal(&subHelmfileSpecTmp); err != nil {
			return err
		}
		hf.Path = subHelmfileSpecTmp.Path
		hf.Selectors = subHelmfileSpecTmp.Selectors
		hf.SelectorsInherited = subHelmfileSpecTmp.SelectorsInherited
		hf.Environment = subHelmfileSpecTmp.Environment
	}
	//since we cannot make sur the "console" string can be red after the "path" we must check we don't have
	//a SubHelmfileSpec with only selector and no path
	if hf.Selectors != nil && hf.Path == "" {
		return fmt.Errorf("found 'selectors' definition without path: %v", hf.Selectors)
	}
	//also exclude SelectorsInherited to true and explicit selectors
	if hf.SelectorsInherited && len(hf.Selectors) > 0 {
		return fmt.Errorf("You cannot use 'SelectorsInherited: true' along with and explicit selector for path: %v", hf.Path)
	}
	return nil
}
