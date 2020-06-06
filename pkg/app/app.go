package app

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"

	"github.com/roboll/helmfile/pkg/argparser"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/plugins"
	"github.com/roboll/helmfile/pkg/remote"
	"github.com/roboll/helmfile/pkg/state"
	"github.com/variantdev/vals"
	"go.uber.org/zap"
)

type App struct {
	OverrideKubeContext string
	OverrideHelmBinary  string

	Logger      *zap.SugaredLogger
	Env         string
	Namespace   string
	Selectors   []string
	Args        string
	ValuesFiles []string
	Set         map[string]interface{}

	FileOrDir string

	readFile          func(string) ([]byte, error)
	fileExists        func(string) (bool, error)
	glob              func(string) ([]string, error)
	abs               func(string) (string, error)
	fileExistsAt      func(string) bool
	directoryExistsAt func(string) bool

	getwd func() (string, error)
	chdir func(string) error

	remote *remote.Remote

	valsRuntime vals.Evaluator

	helms      map[helmKey]helmexec.Interface
	helmsMutex sync.Mutex
}

type HelmRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Enabled   bool   `json:"enabled"`
	Labels    string `json:"labels"`
}

func New(conf ConfigProvider) *App {
	return Init(&App{
		OverrideKubeContext: conf.KubeContext(),
		OverrideHelmBinary:  conf.HelmBinary(),
		Logger:              conf.Logger(),
		Env:                 conf.Env(),
		Namespace:           conf.Namespace(),
		Selectors:           conf.Selectors(),
		Args:                conf.Args(),
		FileOrDir:           conf.FileOrDir(),
		ValuesFiles:         conf.StateValuesFiles(),
		Set:                 conf.StateValuesSet(),
		//helmExecer: helmexec.New(conf.HelmBinary(), conf.Logger(), conf.KubeContext(), &helmexec.ShellRunner{
		//	Logger: conf.Logger(),
		//}),
	})
}

func Init(app *App) *App {
	app.readFile = ioutil.ReadFile
	app.glob = filepath.Glob
	app.abs = filepath.Abs
	app.getwd = os.Getwd
	app.chdir = os.Chdir
	app.fileExistsAt = fileExistsAt
	app.fileExists = fileExists
	app.directoryExistsAt = directoryExistsAt

	var err error
	app.valsRuntime, err = plugins.ValsInstance()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize vals runtime: %v", err))
	}

	return app
}

func (a *App) Deps(c DepsConfigProvider) error {
	return a.ForEachStateFiltered(func(run *Run) (errs []error) {
		prepErrs := run.withReposAndPreparedCharts(false, c.SkipRepos(), func() {
			errs = run.Deps(c)
		})

		errs = append(errs, prepErrs...)

		return
	})
}

func (a *App) Repos(c ReposConfigProvider) error {
	return a.ForEachStateFiltered(func(run *Run) (errs []error) {
		err := run.withPreparedCharts(false, func() {
			errs = run.Repos(c)
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	})
}

func (a *App) DeprecatedSyncCharts(c DeprecatedChartsConfigProvider) error {
	return a.ForEachStateFiltered(func(run *Run) (errs []error) {
		err := run.withPreparedCharts(false, func() {
			errs = run.DeprecatedSyncCharts(c)
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	})
}

func (a *App) Diff(c DiffConfigProvider) error {
	var allDiffDetectedErrs []error

	var affectedAny bool

	err := a.ForEachState(func(run *Run) (bool, []error) {
		var criticalErrs []error

		var msg *string

		var matched, affected bool

		var errs []error

		prepErrs := run.withReposAndPreparedCharts(false, c.SkipDeps(), func() {
			msg, matched, affected, errs = run.Diff(c)
		})

		if msg != nil {
			a.Logger.Info(*msg)
		}

		errs = append(errs, prepErrs...)

		affectedAny = affectedAny || affected

		for i := range errs {
			switch e := errs[i].(type) {
			case *state.ReleaseError:
				switch e.Code {
				case 2:
					// See https://github.com/roboll/helmfile/issues/874
					allDiffDetectedErrs = append(allDiffDetectedErrs, e)
				default:
					criticalErrs = append(criticalErrs, e)
				}
			default:
				criticalErrs = append(criticalErrs, e)
			}
		}

		return matched, criticalErrs
	})

	if err != nil {
		return err
	}

	if c.DetailedExitcode() && (len(allDiffDetectedErrs) > 0 || affectedAny) {
		// We take the first release error w/ exit status 2 (although all the defered errs should have exit status 2)
		// to just let helmfile itself to exit with 2
		// See https://github.com/roboll/helmfile/issues/749
		code := 2
		e := &Error{
			msg:  "Identified at least one change",
			code: &code,
		}
		return e
	}

	return nil
}

func (a *App) Template(c TemplateConfigProvider) error {
	return a.ForEachState(func(run *Run) (ok bool, errs []error) {
		prepErrs := run.withReposAndPreparedCharts(true, c.SkipDeps(), func() {
			ok, errs = a.template(run, c)
		})

		errs = append(errs, prepErrs...)

		return
	})
}

func (a *App) Lint(c LintConfigProvider) error {
	return a.ForEachStateFiltered(func(run *Run) (errs []error) {
		prepErrs := run.withReposAndPreparedCharts(true, c.SkipDeps(), func() {
			errs = run.Lint(c)
		})

		errs = append(errs, prepErrs...)

		return
	})
}

func (a *App) Sync(c SyncConfigProvider) error {
	return a.ForEachState(func(run *Run) (ok bool, errs []error) {
		prepErrs := run.withReposAndPreparedCharts(false, c.SkipDeps(), func() {
			ok, errs = a.sync(run, c)
		})

		errs = append(errs, prepErrs...)

		return
	})
}

func (a *App) Apply(c ApplyConfigProvider) error {
	var any bool

	mut := &sync.Mutex{}

	var opts []LoadOption

	opts = append(opts, SetRetainValuesFiles(c.RetainValuesFiles()))

	err := a.ForEachState(func(run *Run) (ok bool, errs []error) {
		prepErrs := run.withReposAndPreparedCharts(false, c.SkipDeps(), func() {
			matched, updated, es := a.apply(run, c)

			mut.Lock()
			any = any || updated
			mut.Unlock()

			ok, errs = matched, es
		})

		errs = append(errs, prepErrs...)

		return
	}, opts...)

	if err != nil {
		return err
	}

	if c.DetailedExitcode() && any {
		code := 2

		return &Error{msg: "", Errors: nil, code: &code}
	}

	return nil
}

func (a *App) Status(c StatusesConfigProvider) error {
	return a.ForEachStateFiltered(func(run *Run) (errs []error) {
		err := run.withPreparedCharts(false, func() {
			errs = run.Status(c)
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	})
}

func (a *App) Delete(c DeleteConfigProvider) error {
	return a.ForEachState(func(run *Run) (ok bool, errs []error) {
		err := run.withPreparedCharts(false, func() {
			ok, errs = a.delete(run, c.Purge(), c)
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	}, SetReverse(true))
}

func (a *App) Destroy(c DestroyConfigProvider) error {
	return a.ForEachState(func(run *Run) (ok bool, errs []error) {
		err := run.withPreparedCharts(false, func() {
			ok, errs = a.delete(run, true, c)
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	}, SetReverse(true))
}

func (a *App) Test(c TestConfigProvider) error {
	return a.ForEachStateFiltered(func(run *Run) (errs []error) {
		if c.Cleanup() && run.helm.IsHelm3() {
			a.Logger.Warnf("warn: requested cleanup will not be applied. " +
				"To clean up test resources with Helm 3, you have to remove them manually " +
				"or set helm.sh/hook-delete-policy\n")
		}

		err := run.withPreparedCharts(false, func() {
			errs = run.Test(c)
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	})
}

func (a *App) PrintState(c StateConfigProvider) error {
	return a.VisitDesiredStatesWithReleasesFiltered(a.FileOrDir, func(st *state.HelmState) (errs []error) {
		err := NewRun(st, nil, NewContext()).withPreparedCharts(false, func() {
			state, err := st.ToYaml()
			if err != nil {
				errs = []error{err}
				return
			}
			fmt.Printf("---\n#  Source: %s\n\n%+v", st.FilePath, state)

			errs = []error{}
		})

		if err != nil {
			errs = append(errs, err)
		}

		return
	})
}

func (a *App) ListReleases(c ListConfigProvider) error {
	var releases []*HelmRelease

	err := a.VisitDesiredStatesWithReleasesFiltered(a.FileOrDir, func(st *state.HelmState) []error {
		err := NewRun(st, nil, NewContext()).withPreparedCharts(false, func() {

			//var releases m
			for _, r := range st.Releases {
				labels := ""
				for k, v := range r.Labels {
					labels = fmt.Sprintf("%s,%s:%s", labels, k, v)
				}
				labels = strings.Trim(labels, ",")
				installed := r.Installed == nil || *r.Installed
				releases = append(releases, &HelmRelease{
					Name:      r.Name,
					Namespace: r.Namespace,
					Enabled:   installed,
					Labels:    labels,
				})
			}
		})

		var errs []error

		if err != nil {
			errs = append(errs, err)
		}

		return errs
	})

	if err != nil {
		return err
	}

	if c.Output() == "json" {
		err = FormatAsJson(releases)
	} else {
		err = FormatAsTable(releases)
	}

	return err
}

func (a *App) within(dir string, do func() error) error {
	if dir == "." {
		return do()
	}

	prev, err := a.getwd()
	if err != nil {
		return fmt.Errorf("failed getting current working direcotyr: %v", err)
	}

	absDir, err := a.abs(dir)
	if err != nil {
		return err
	}

	a.Logger.Debugf("changing working directory to \"%s\"", absDir)

	if err := a.chdir(absDir); err != nil {
		return fmt.Errorf("failed changing working directory to \"%s\": %v", absDir, err)
	}

	appErr := do()

	a.Logger.Debugf("changing working directory back to \"%s\"", prev)

	if chdirBackErr := a.chdir(prev); chdirBackErr != nil {
		if appErr != nil {
			a.Logger.Warnf("%v", appErr)
		}
		return fmt.Errorf("failed chaging working directory back to \"%s\": %v", prev, chdirBackErr)
	}

	return appErr
}

func (a *App) visitStateFiles(fileOrDir string, opts LoadOpts, do func(string, string) error) error {
	desiredStateFiles, err := a.findDesiredStateFiles(fileOrDir, opts)
	if err != nil {
		return appError("", err)
	}

	for _, relPath := range desiredStateFiles {
		var file string
		var dir string
		if a.directoryExistsAt(relPath) {
			file = relPath
			dir = relPath
		} else {
			file = filepath.Base(relPath)
			dir = filepath.Dir(relPath)
		}

		a.Logger.Debugf("processing file \"%s\" in directory \"%s\"", file, dir)

		err := a.within(dir, func() error {
			absd, err := a.abs(dir)
			if err != nil {
				return err
			}

			return do(file, absd)
		})
		if err != nil {
			return appError(fmt.Sprintf("in %s/%s", dir, file), err)
		}
	}

	return nil
}

func (a *App) loadDesiredStateFromYaml(file string, opts ...LoadOpts) (*state.HelmState, error) {
	var op LoadOpts
	if len(opts) > 0 {
		op = opts[0]
	}

	ld := &desiredStateLoader{
		readFile:   a.readFile,
		fileExists: a.fileExists,
		env:        a.Env,
		namespace:  a.Namespace,
		logger:     a.Logger,
		abs:        a.abs,

		overrideKubeContext: a.OverrideKubeContext,
		overrideHelmBinary:  a.OverrideHelmBinary,
		glob:                a.glob,
		getHelm:             a.getHelm,
		valsRuntime:         a.valsRuntime,
	}

	return ld.Load(file, op)
}

type helmKey struct {
	Binary  string
	Context string
}

func createHelmKey(bin, kubectx string) helmKey {
	return helmKey{
		Binary:  bin,
		Context: kubectx,
	}
}

// GetHelm returns the global helm exec instance for the specified state that is used for helmfile-wise operation
// like decrypting environment secrets.
//
// This is currently used for running all the helm commands for reconciling releases. But this may change in the future
// once we enable each release to have its own helm binary/version.
func (a *App) getHelm(st *state.HelmState) helmexec.Interface {
	a.helmsMutex.Lock()
	defer a.helmsMutex.Unlock()

	if a.helms == nil {
		a.helms = map[helmKey]helmexec.Interface{}
	}

	bin := st.DefaultHelmBinary
	kubectx := st.HelmDefaults.KubeContext

	key := createHelmKey(bin, kubectx)

	if _, ok := a.helms[key]; !ok {
		a.helms[key] = helmexec.New(bin, a.Logger, kubectx, &helmexec.ShellRunner{
			Logger: a.Logger,
		})
	}

	return a.helms[key]
}

func (a *App) visitStates(fileOrDir string, defOpts LoadOpts, converge func(*state.HelmState) (bool, []error)) error {
	noMatchInHelmfiles := true

	err := a.visitStateFiles(fileOrDir, defOpts, func(f, d string) error {
		opts := defOpts.DeepCopy()

		if opts.CalleePath == "" {
			opts.CalleePath = f
		}

		st, err := a.loadDesiredStateFromYaml(f, opts)

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigs

			errs := []error{fmt.Errorf("Received [%s] to shutdown ", sig)}
			_ = context{app: a, st: st, retainValues: defOpts.RetainValuesFiles}.clean(errs)
			// See http://tldp.org/LDP/abs/html/exitcodes.html
			switch sig {
			case syscall.SIGINT:
				os.Exit(130)
			case syscall.SIGTERM:
				os.Exit(143)
			}
		}()

		ctx := context{app: a, st: st, retainValues: defOpts.RetainValuesFiles}

		if err != nil {
			switch stateLoadErr := err.(type) {
			// Addresses https://github.com/roboll/helmfile/issues/279
			case *state.StateLoadError:
				switch stateLoadErr.Cause.(type) {
				case *state.UndefinedEnvError:
					return nil
				default:
					return ctx.wrapErrs(err)
				}
			default:
				return ctx.wrapErrs(err)
			}
		}
		st.Selectors = opts.Selectors

		visitSubHelmfiles := func() error {
			if len(st.Helmfiles) > 0 {
				noMatchInSubHelmfiles := true
				for i, m := range st.Helmfiles {
					optsForNestedState := LoadOpts{
						CalleePath:        filepath.Join(d, f),
						Environment:       m.Environment,
						Reverse:           defOpts.Reverse,
						RetainValuesFiles: defOpts.RetainValuesFiles,
					}
					//assign parent selector to sub helm selector in legacy mode or do not inherit in experimental mode
					if (m.Selectors == nil && !isExplicitSelectorInheritanceEnabled()) || m.SelectorsInherited {
						optsForNestedState.Selectors = opts.Selectors
					} else {
						optsForNestedState.Selectors = m.Selectors
					}

					if err := a.visitStates(m.Path, optsForNestedState, converge); err != nil {
						switch err.(type) {
						case *NoMatchingHelmfileError:

						default:
							return appError(fmt.Sprintf("in .helmfiles[%d]", i), err)
						}
					} else {
						noMatchInSubHelmfiles = false
					}
				}
				noMatchInHelmfiles = noMatchInHelmfiles && noMatchInSubHelmfiles
			}
			return nil
		}

		if !opts.Reverse {
			err = visitSubHelmfiles()
			if err != nil {
				return err
			}
		}

		templated, tmplErr := st.ExecuteTemplates()
		if tmplErr != nil {
			return appError(fmt.Sprintf("failed executing release templates in \"%s\"", f), tmplErr)
		}

		processed, errs := converge(templated)
		noMatchInHelmfiles = noMatchInHelmfiles && !processed

		if opts.Reverse {
			err = visitSubHelmfiles()
			if err != nil {
				return err
			}
		}

		return context{app: a, st: templated, retainValues: defOpts.RetainValuesFiles}.clean(errs)
	})

	if err != nil {
		return err
	}

	if noMatchInHelmfiles {
		return &NoMatchingHelmfileError{selectors: a.Selectors, env: a.Env}
	}

	return nil
}

func (a *App) ForEachStateFiltered(do func(*Run) []error) error {
	ctx := NewContext()

	err := a.VisitDesiredStatesWithReleasesFiltered(a.FileOrDir, func(st *state.HelmState) []error {
		helm := a.getHelm(st)

		run := NewRun(st, helm, ctx)

		return do(run)
	})

	return err
}

type LoadOption func(o *LoadOpts)

var (
	SetReverse = func(r bool) func(o *LoadOpts) {
		return func(o *LoadOpts) {
			o.Reverse = r
		}
	}

	SetRetainValuesFiles = func(r bool) func(o *LoadOpts) {
		return func(o *LoadOpts) {
			o.RetainValuesFiles = true
		}
	}
)

func (a *App) ForEachState(do func(*Run) (bool, []error), o ...LoadOption) error {
	ctx := NewContext()
	err := a.visitStatesWithSelectorsAndRemoteSupport(a.FileOrDir, func(st *state.HelmState) (bool, []error) {
		helm := a.getHelm(st)

		run := NewRun(st, helm, ctx)
		return do(run)
	}, o...)

	return err
}

func printBatches(batches [][]state.Release) string {
	buf := &bytes.Buffer{}

	w := new(tabwriter.Writer)

	w.Init(buf, 0, 1, 1, ' ', 0)

	fmt.Fprintln(w, "GROUP\tRELEASES")

	for i, batch := range batches {
		ids := []string{}
		for _, r := range batch {
			ids = append(ids, state.ReleaseToID(&r.ReleaseSpec))
		}
		fmt.Fprintf(w, "%d\t%s\n", i+1, strings.Join(ids, ", "))
	}

	w.Flush()

	return buf.String()
}

func withDAG(templated *state.HelmState, helm helmexec.Interface, logger *zap.SugaredLogger, reverse bool, converge func(*state.HelmState, helmexec.Interface) (bool, []error)) (bool, []error) {
	batches, err := templated.PlanReleases(reverse)
	if err != nil {
		return false, []error{err}
	}

	numBatches := len(batches)

	logger.Debugf("processing %d groups of releases in this order:\n%s", numBatches, printBatches(batches))

	any := false

	for i, batch := range batches {
		var targets []state.ReleaseSpec

		for _, marked := range batch {
			targets = append(targets, marked.ReleaseSpec)
		}

		var releaseIds []string
		for _, r := range targets {
			releaseIds = append(releaseIds, state.ReleaseToID(&r))
		}

		logger.Debugf("processing releases in group %d/%d: %s", i+1, numBatches, strings.Join(releaseIds, ", "))

		batchSt := *templated
		batchSt.Releases = targets

		processed, errs := converge(&batchSt, helm)

		if errs != nil && len(errs) > 0 {
			return false, errs
		}

		any = any || processed
	}

	return any, nil
}

type Opts struct {
	DAGEnabled bool
}

func (a *App) visitStatesWithSelectorsAndRemoteSupport(fileOrDir string, converge func(*state.HelmState) (bool, []error), opt ...LoadOption) error {
	opts := LoadOpts{
		Selectors: a.Selectors,
	}

	for _, o := range opt {
		o(&opts)
	}

	envvals := []interface{}{}

	if a.ValuesFiles != nil {
		for i := range a.ValuesFiles {
			envvals = append(envvals, a.ValuesFiles[i])
		}
	}

	if a.Set != nil {
		envvals = append(envvals, a.Set)
	}

	if len(envvals) > 0 {
		opts.Environment.OverrideValues = envvals
	}

	dir, err := a.getwd()
	if err != nil {
		return err
	}

	getter := &remote.GoGetter{Logger: a.Logger}

	remote := &remote.Remote{
		Logger:     a.Logger,
		Home:       dir,
		Getter:     getter,
		ReadFile:   a.readFile,
		DirExists:  a.directoryExistsAt,
		FileExists: a.fileExistsAt,
	}

	a.remote = remote

	return a.visitStates(fileOrDir, opts, converge)
}

func processFilteredReleases(st *state.HelmState, helm helmexec.Interface, converge func(st *state.HelmState) []error) (bool, []error) {
	if len(st.Selectors) > 0 {
		err := st.FilterReleases()
		if err != nil {
			return false, []error{err}
		}
	}

	type Key struct {
		TillerNamespace, Name string
	}

	releaseNameCounts := map[Key]int{}
	for _, r := range st.Releases {
		namespace := r.Namespace
		if !helm.IsHelm3() {
			if r.TillerNamespace != "" {
				namespace = r.TillerNamespace
			} else {
				namespace = st.HelmDefaults.TillerNamespace
			}
		}
		releaseNameCounts[Key{namespace, r.Name}]++
	}
	for name, c := range releaseNameCounts {
		if c > 1 {
			return false, []error{fmt.Errorf("duplicate release \"%s\" found in \"%s\": there were %d releases named \"%s\" matching specified selector", name.Name, name.TillerNamespace, c, name.Name)}
		}
	}

	errs := converge(st)

	processed := len(st.Releases) != 0 && len(errs) == 0

	return processed, errs
}

func (a *App) Wrap(converge func(*state.HelmState, helmexec.Interface) []error) func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
	return func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
		return processFilteredReleases(st, helm, func(st *state.HelmState) []error {
			return converge(st, helm)
		})
	}
}

func (a *App) VisitDesiredStatesWithReleasesFiltered(fileOrDir string, converge func(*state.HelmState) []error, o ...LoadOption) error {
	return a.visitStatesWithSelectorsAndRemoteSupport(fileOrDir, func(st *state.HelmState) (bool, []error) {
		return processFilteredReleases(st, a.getHelm(st), converge)
	}, o...)
}

func (a *App) findDesiredStateFiles(specifiedPath string, opts LoadOpts) ([]string, error) {
	path, err := a.remote.Locate(specifiedPath)
	if err != nil {
		return nil, fmt.Errorf("locate: %v", err)
	}
	if specifiedPath != path {
		a.Logger.Debugf("fetched remote \"%s\" to local cache \"%s\" and loading the latter...", specifiedPath, path)
	}
	specifiedPath = path

	var helmfileDir string
	if specifiedPath != "" {
		if a.fileExistsAt(specifiedPath) {
			return []string{specifiedPath}, nil
		} else if a.directoryExistsAt(specifiedPath) {
			helmfileDir = specifiedPath
		} else {
			return []string{}, fmt.Errorf("specified state file %s is not found", specifiedPath)
		}
	} else {
		var defaultFile string
		if a.fileExistsAt(DefaultHelmfile) {
			defaultFile = DefaultHelmfile
		} else if a.fileExistsAt(DeprecatedHelmfile) {
			log.Printf(
				"warn: %s is being loaded: %s is deprecated in favor of %s. See https://github.com/roboll/helmfile/issues/25 for more information",
				DeprecatedHelmfile,
				DeprecatedHelmfile,
				DefaultHelmfile,
			)
			defaultFile = DeprecatedHelmfile
		}

		if a.directoryExistsAt(DefaultHelmfileDirectory) {
			if defaultFile != "" {
				return []string{}, fmt.Errorf("configuration conlict error: you can have either %s or %s, but not both", defaultFile, DefaultHelmfileDirectory)
			}

			helmfileDir = DefaultHelmfileDirectory
		} else if defaultFile != "" {
			return []string{defaultFile}, nil
		} else {
			return []string{}, fmt.Errorf("no state file found. It must be named %s/*.{yaml,yml}, %s, or %s, or otherwise specified with the --file flag", DefaultHelmfileDirectory, DefaultHelmfile, DeprecatedHelmfile)
		}
	}

	files, err := a.glob(filepath.Join(helmfileDir, "*.y*ml"))
	if err != nil {
		return []string{}, err
	}
	if opts.Reverse {
		sort.Slice(files, func(i, j int) bool {
			return files[j] < files[i]
		})
	} else {
		sort.Slice(files, func(i, j int) bool {
			return files[i] < files[j]
		})
	}
	return files, nil
}

func (a *App) apply(r *Run, c ApplyConfigProvider) (bool, bool, []error) {
	st := r.state
	helm := r.helm

	allReleases := st.GetReleasesWithOverrides()

	toApply, err := st.GetSelectedReleasesWithOverrides()
	if err != nil {
		return false, false, []error{err}
	}
	if len(toApply) == 0 {
		return false, false, nil
	}

	// Do build deps and prepare only on selected releases so that we won't waste time
	// on running various helm commands on unnecessary releases
	st.Releases = toApply

	if !c.SkipDeps() {
		if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
			return false, false, errs
		}
	}
	if errs := st.PrepareReleases(helm, "apply"); errs != nil && len(errs) > 0 {
		return false, false, errs
	}

	// helm must be 2.11+ and helm-diff should be provided `--detailed-exitcode` in order for `helmfile apply` to work properly
	detailedExitCode := true

	diffOpts := &state.DiffOpts{
		NoColor: c.NoColor(),
		Context: c.Context(),
		Set:     c.Set(),
	}

	infoMsg, releasesToBeUpdated, releasesToBeDeleted, errs := r.diff(false, detailedExitCode, c, diffOpts)
	if len(errs) > 0 {
		return false, false, errs
	}

	if releasesToBeDeleted == nil && releasesToBeUpdated == nil {
		if infoMsg != nil {
			logger := c.Logger()
			logger.Infof("")
			logger.Infof(*infoMsg)
		}
		return true, false, nil
	}

	confMsg := fmt.Sprintf(`%s
Do you really want to apply?
  Helmfile will apply all your changes, as shown above.

`, *infoMsg)
	interactive := c.Interactive()
	if !interactive {
		a.Logger.Debug(*infoMsg)
	}

	syncErrs := []error{}

	affectedReleases := state.AffectedReleases{}

	// Traverse DAG of all the releases so that we don't suffer from false-positive missing dependencies
	st.Releases = allReleases

	if !interactive || interactive && r.askForConfirmation(confMsg) {
		r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

		// We deleted releases by traversing the DAG in reverse order
		if len(releasesToBeDeleted) > 0 {
			_, deletionErrs := withDAG(st, helm, a.Logger, true, a.Wrap(func(subst *state.HelmState, helm helmexec.Interface) []error {
				var rs []state.ReleaseSpec

				for _, r := range subst.Releases {
					if r2, ok := releasesToBeDeleted[state.ReleaseToID(&r)]; ok {
						rs = append(rs, r2)
					}
				}

				subst.Releases = rs

				return subst.DeleteReleasesForSync(&affectedReleases, helm, c.Concurrency())
			}))

			if deletionErrs != nil && len(deletionErrs) > 0 {
				syncErrs = append(syncErrs, deletionErrs...)
			}
		}

		// We upgrade releases by traversing the DAG
		if len(releasesToBeUpdated) > 0 {
			_, updateErrs := withDAG(st, helm, a.Logger, false, a.Wrap(func(subst *state.HelmState, helm helmexec.Interface) []error {
				var rs []state.ReleaseSpec

				for _, r := range subst.Releases {
					if r2, ok := releasesToBeUpdated[state.ReleaseToID(&r)]; ok {
						rs = append(rs, r2)
					}
				}

				subst.Releases = rs

				syncOpts := state.SyncOpts{
					Set: c.Set(),
				}
				return subst.SyncReleases(&affectedReleases, helm, c.Values(), c.Concurrency(), &syncOpts)
			}))

			if updateErrs != nil && len(updateErrs) > 0 {
				syncErrs = append(syncErrs, updateErrs...)
			}
		}
	}

	affectedReleases.DisplayAffectedReleases(c.Logger())
	return true, true, syncErrs
}

func (a *App) delete(r *Run, purge bool, c DestroyConfigProvider) (bool, []error) {
	st := r.state
	helm := r.helm

	affectedReleases := state.AffectedReleases{}

	toSync, err := st.GetSelectedReleasesWithOverrides()
	if err != nil {
		return false, []error{err}
	}
	if len(toSync) == 0 {
		return false, nil
	}

	toDelete, err := st.DetectReleasesToBeDeleted(helm, toSync)
	if err != nil {
		return false, []error{err}
	}

	releasesToDelete := map[string]state.ReleaseSpec{}
	for _, r := range toDelete {
		id := state.ReleaseToID(&r)
		releasesToDelete[id] = r
	}

	names := make([]string, len(toSync))
	for i, r := range toSync {
		names[i] = fmt.Sprintf("  %s (%s)", r.Name, r.Chart)
	}

	var errs []error

	msg := fmt.Sprintf(`Affected releases are:
%s

Do you really want to delete?
  Helmfile will delete all your releases, as shown above.

`, strings.Join(names, "\n"))
	interactive := c.Interactive()
	if !interactive || interactive && r.askForConfirmation(msg) {
		r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

		if len(releasesToDelete) > 0 {
			_, deletionErrs := withDAG(st, helm, a.Logger, true, a.Wrap(func(subst *state.HelmState, helm helmexec.Interface) []error {
				var rs []state.ReleaseSpec

				for _, r := range subst.Releases {
					if _, ok := releasesToDelete[state.ReleaseToID(&r)]; ok {
						rs = append(rs, r)
					}
				}

				subst.Releases = rs

				return subst.DeleteReleases(&affectedReleases, helm, c.Concurrency(), purge)
			}))

			if deletionErrs != nil && len(deletionErrs) > 0 {
				errs = append(errs, deletionErrs...)
			}
		}
	}
	affectedReleases.DisplayAffectedReleases(c.Logger())
	return true, errs
}

func (a *App) sync(r *Run, c SyncConfigProvider) (bool, []error) {
	st := r.state
	helm := r.helm

	allReleases := st.GetReleasesWithOverrides()

	toSync, err := st.GetSelectedReleasesWithOverrides()
	if err != nil {
		return false, []error{err}
	}
	if len(toSync) == 0 {
		return false, nil
	}

	// Do build deps and prepare only on selected releases so that we won't waste time
	// on running various helm commands on unnecessary releases
	st.Releases = toSync

	if !c.SkipDeps() {
		if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
			return false, errs
		}
	}
	if errs := st.PrepareReleases(helm, "sync"); errs != nil && len(errs) > 0 {
		return false, errs
	}

	toDelete, err := st.DetectReleasesToBeDeletedForSync(helm, toSync)
	if err != nil {
		return false, []error{err}
	}

	releasesToDelete := map[string]state.ReleaseSpec{}
	for _, r := range toDelete {
		id := state.ReleaseToID(&r)
		releasesToDelete[id] = r
	}

	var toUpdate []state.ReleaseSpec
	for _, r := range toSync {
		if _, deleted := releasesToDelete[state.ReleaseToID(&r)]; !deleted {
			toUpdate = append(toUpdate, r)
		}
	}

	releasesToUpdate := map[string]state.ReleaseSpec{}
	for _, r := range toUpdate {
		id := state.ReleaseToID(&r)
		releasesToUpdate[id] = r
	}

	names := []string{}
	for _, r := range releasesToUpdate {
		names = append(names, fmt.Sprintf("  %s (%s) UPDATED", r.Name, r.Chart))
	}
	for _, r := range releasesToDelete {
		names = append(names, fmt.Sprintf("  %s (%s) DELETED", r.Name, r.Chart))
	}
	// Make the output deterministic for testing purpose
	sort.Strings(names)

	infoMsg := fmt.Sprintf(`Affected releases are:
%s
`, strings.Join(names, "\n"))

	a.Logger.Info(infoMsg)

	var errs []error

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	// Traverse DAG of all the releases so that we don't suffer from false-positive missing dependencies
	st.Releases = allReleases

	affectedReleases := state.AffectedReleases{}

	if len(releasesToDelete) > 0 {
		_, deletionErrs := withDAG(st, helm, a.Logger, true, a.Wrap(func(subst *state.HelmState, helm helmexec.Interface) []error {
			var rs []state.ReleaseSpec

			for _, r := range subst.Releases {
				if r2, ok := releasesToDelete[state.ReleaseToID(&r)]; ok {
					rs = append(rs, r2)
				}
			}

			subst.Releases = rs

			return subst.DeleteReleasesForSync(&affectedReleases, helm, c.Concurrency())
		}))

		if deletionErrs != nil && len(deletionErrs) > 0 {
			errs = append(errs, deletionErrs...)
		}
	}

	if len(releasesToUpdate) > 0 {
		_, syncErrs := withDAG(st, helm, a.Logger, false, a.Wrap(func(subst *state.HelmState, helm helmexec.Interface) []error {
			var rs []state.ReleaseSpec

			for _, r := range subst.Releases {
				if r2, ok := releasesToUpdate[state.ReleaseToID(&r)]; ok {
					rs = append(rs, r2)
				}
			}

			subst.Releases = rs

			opts := &state.SyncOpts{
				Set: c.Set(),
			}
			return subst.SyncReleases(&affectedReleases, helm, c.Values(), c.Concurrency(), opts)
		}))

		if syncErrs != nil && len(syncErrs) > 0 {
			errs = append(errs, syncErrs...)
		}
	}
	affectedReleases.DisplayAffectedReleases(c.Logger())
	return true, errs
}

func (a *App) template(r *Run, c TemplateConfigProvider) (bool, []error) {
	st := r.state
	helm := r.helm

	allReleases := st.GetReleasesWithOverrides()

	toRender, err := st.GetSelectedReleasesWithOverrides()
	if err != nil {
		return false, []error{err}
	}
	if len(toRender) == 0 {
		return false, nil
	}

	// Do build deps and prepare only on selected releases so that we won't waste time
	// on running various helm commands on unnecessary releases
	st.Releases = toRender

	if !c.SkipDeps() {
		if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
			return false, errs
		}
	}
	if errs := st.PrepareReleases(helm, "template"); errs != nil && len(errs) > 0 {
		return false, errs
	}

	releasesToRender := map[string]state.ReleaseSpec{}
	for _, r := range toRender {
		id := state.ReleaseToID(&r)
		if r.Installed != nil && !*r.Installed {
			continue
		}
		releasesToRender[id] = r
	}

	names := make([]string, len(toRender))
	for i, r := range toRender {
		names[i] = fmt.Sprintf("  %s (%s)", r.Name, r.Chart)
	}

	var errs []error

	// Traverse DAG of all the releases so that we don't suffer from false-positive missing dependencies
	st.Releases = allReleases

	if len(releasesToRender) > 0 {
		_, templateErrs := withDAG(st, helm, a.Logger, false, a.Wrap(func(subst *state.HelmState, helm helmexec.Interface) []error {
			var rs []state.ReleaseSpec

			for _, r := range subst.Releases {
				if r2, ok := releasesToRender[state.ReleaseToID(&r)]; ok {
					rs = append(rs, r2)
				}
			}

			subst.Releases = rs

			args := argparser.GetArgs(c.Args(), st)
			opts := &state.TemplateOpts{
				Set: c.Set(),
			}
			return subst.TemplateReleases(helm, c.OutputDir(), c.Values(), args, c.Concurrency(), c.Validate(), opts)
		}))

		if templateErrs != nil && len(templateErrs) > 0 {
			errs = append(errs, templateErrs...)
		}
	}
	return true, errs
}

func fileExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsRegular()
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func directoryExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsDir()
}

type Error struct {
	msg string

	Errors []error

	code *int
}

func (e *Error) Error() string {
	var cause string
	if e.Errors == nil {
		return e.msg
	}
	if len(e.Errors) == 1 {
		if e.Errors[0] == nil {
			panic(fmt.Sprintf("[bug] assertion error: unexpected state: e.Errors: %v", e.Errors))
		}
		cause = e.Errors[0].Error()
	} else {
		msgs := []string{}
		for i, err := range e.Errors {
			if err == nil {
				continue
			}
			msgs = append(msgs, fmt.Sprintf("err %d: %v", i, err.Error()))
		}
		cause = fmt.Sprintf("%d errors:\n%s", len(e.Errors), strings.Join(msgs, "\n"))
	}
	msg := ""
	if e.msg != "" {
		msg = fmt.Sprintf("%s: %s", e.msg, cause)
	} else {
		msg = cause
	}
	return msg
}

func (e *Error) Code() int {
	if e.code != nil {
		return *e.code
	}

	allDiff := false
	anyNonZero := false
	for _, err := range e.Errors {
		switch ee := err.(type) {
		case *state.ReleaseError:
			if anyNonZero {
				allDiff = allDiff && ee.Code == 2
			} else {
				allDiff = ee.Code == 2
			}
		case *Error:
			if anyNonZero {
				allDiff = allDiff && ee.Code() == 2
			} else {
				allDiff = ee.Code() == 2
			}
		}
		anyNonZero = true
	}

	if anyNonZero {
		if allDiff {
			return 2
		}
		return 1
	}
	panic(fmt.Sprintf("[bug] assertion error: unexpected state: unable to handle errors: %v", e.Errors))
}

func appError(msg string, err error) *Error {
	return &Error{msg: msg, Errors: []error{err}}
}

func (c context) clean(errs []error) error {
	if errs == nil {
		errs = []error{}
	}

	if !c.retainValues {
		cleanErrs := c.st.Clean()
		if cleanErrs != nil {
			errs = append(errs, cleanErrs...)
		}
	}

	return c.wrapErrs(errs...)
}

type context struct {
	app *App
	st  *state.HelmState

	retainValues bool
}

func (c context) wrapErrs(errs ...error) error {
	if errs != nil && len(errs) > 0 {
		for _, err := range errs {
			switch e := err.(type) {
			case *state.ReleaseError:
				c.app.Logger.Debugf("err: release \"%s\" in \"%s\" failed: %v", e.Name, c.st.FilePath, e)
			default:
				c.app.Logger.Debugf("err: %v", e)
			}
		}
		return &Error{Errors: errs}
	}
	return nil
}
