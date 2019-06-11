package app

import (
	"fmt"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/remote"
	"github.com/roboll/helmfile/pkg/state"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.uber.org/zap"

	"path/filepath"
	"sort"
)

type App struct {
	KubeContext string
	Logger      *zap.SugaredLogger
	Reverse     bool
	Env         string
	Namespace   string
	Selectors   []string
	HelmBinary  string
	Args        string
	ValuesFiles []string
	Set         map[string]interface{}

	FileOrDir string

	ErrorHandler func(error) error

	readFile          func(string) ([]byte, error)
	fileExists        func(string) (bool, error)
	glob              func(string) ([]string, error)
	abs               func(string) (string, error)
	fileExistsAt      func(string) bool
	directoryExistsAt func(string) bool

	getwd func() (string, error)
	chdir func(string) error

	remote *remote.Remote
}

func New(conf ConfigProvider) *App {
	return Init(&App{
		KubeContext: conf.KubeContext(),
		Logger:      conf.Logger(),
		Env:         conf.Env(),
		Namespace:   conf.Namespace(),
		Selectors:   conf.Selectors(),
		HelmBinary:  conf.HelmBinary(),
		Args:        conf.Args(),
		FileOrDir:   conf.FileOrDir(),
		ValuesFiles: conf.ValuesFiles(),
		Set:         conf.Set(),
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
	return app
}

func (a *App) Deps(c DepsConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Deps(c)
	})
}

func (a *App) Repos(c ReposConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Repos(c)
	})
}

func (a *App) reverse() *App {
	new := *a
	new.Reverse = true
	return &new
}

func (a *App) DeprecatedSyncCharts(c DeprecatedChartsConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.DeprecatedSyncCharts(c)
	})
}

func (a *App) Diff(c DiffConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Diff(c)
	})
}

func (a *App) Template(c TemplateConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Template(c)
	})
}

func (a *App) Lint(c LintConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Lint(c)
	})
}

func (a *App) Sync(c SyncConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Sync(c)
	})
}

func (a *App) Apply(c ApplyConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Apply(c)
	})
}

func (a *App) Status(c StatusesConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Status(c)
	})
}

func (a *App) Delete(c DeleteConfigProvider) error {
	return a.reverse().ForEachState(func(run *Run) []error {
		return run.Delete(c)
	})
}

func (a *App) Destroy(c DestroyConfigProvider) error {
	return a.reverse().ForEachState(func(run *Run) []error {
		return run.Destroy(c)
	})
}

func (a *App) Test(c TestConfigProvider) error {
	return a.ForEachState(func(run *Run) []error {
		return run.Test(c)
	})
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

func (a *App) visitStateFiles(fileOrDir string, do func(string, string) error) error {
	desiredStateFiles, err := a.findDesiredStateFiles(fileOrDir)
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
	ld := &desiredStateLoader{
		readFile:   a.readFile,
		fileExists: a.fileExists,
		env:        a.Env,
		namespace:  a.Namespace,
		logger:     a.Logger,
		abs:        a.abs,

		Reverse:     a.Reverse,
		KubeContext: a.KubeContext,
		glob:        a.glob,
	}

	var op LoadOpts
	if len(opts) > 0 {
		op = opts[0]
	}

	return ld.Load(file, op)
}

func (a *App) visitStates(fileOrDir string, defOpts LoadOpts, converge func(*state.HelmState, helmexec.Interface) (bool, []error)) error {
	noMatchInHelmfiles := true

	err := a.visitStateFiles(fileOrDir, func(f, d string) error {
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
			_ = context{a, st}.clean(errs)
			// See http://tldp.org/LDP/abs/html/exitcodes.html
			switch sig {
			case syscall.SIGINT:
				os.Exit(130)
			case syscall.SIGTERM:
				os.Exit(143)
			}
		}()

		ctx := context{a, st}

		helm := helmexec.New(a.Logger, a.KubeContext)

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

		if len(st.Helmfiles) > 0 {
			noMatchInSubHelmfiles := true
			for i, m := range st.Helmfiles {
				optsForNestedState := LoadOpts{
					CalleePath:  filepath.Join(d, f),
					Environment: m.Environment,
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

		templated, tmplErr := st.ExecuteTemplates()
		if tmplErr != nil {
			return appError(fmt.Sprintf("failed executing release templates in \"%s\"", f), tmplErr)
		}
		processed, errs := converge(templated, helm)
		noMatchInHelmfiles = noMatchInHelmfiles && !processed
		return context{a, templated}.clean(errs)
	})

	if err != nil {
		return err
	}

	if noMatchInHelmfiles {
		return &NoMatchingHelmfileError{selectors: a.Selectors, env: a.Env}
	}

	return nil
}

func (a *App) ForEachState(do func(*Run) []error) error {
	err := a.VisitDesiredStatesWithReleasesFiltered(a.FileOrDir, func(st *state.HelmState, helm helmexec.Interface) []error {
		ctx := NewContext()

		run := NewRun(st, helm, ctx)

		return do(run)
	})

	if err != nil && a.ErrorHandler != nil {
		return a.ErrorHandler(err)
	}

	return err
}

func (a *App) VisitDesiredStatesWithReleasesFiltered(fileOrDir string, converge func(*state.HelmState, helmexec.Interface) []error) error {
	opts := LoadOpts{
		Selectors: a.Selectors,
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

	return a.visitStates(fileOrDir, opts, func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
		if len(st.Selectors) > 0 {
			err := st.FilterReleases()
			if err != nil {
				return false, []error{err}
			}
		}

		if a.HelmBinary != "" {
			helm.SetHelmBinary(a.HelmBinary)
		}

		type Key struct {
			TillerNamespace, Name string
		}

		releaseNameCounts := map[Key]int{}
		for _, r := range st.Releases {
			tillerNamespace := st.HelmDefaults.TillerNamespace
			if r.TillerNamespace != "" {
				tillerNamespace = r.TillerNamespace
			}
			releaseNameCounts[Key{tillerNamespace, r.Name}]++
		}
		for name, c := range releaseNameCounts {
			if c > 1 {
				return false, []error{fmt.Errorf("duplicate release \"%s\" found in \"%s\": there were %d releases named \"%s\" matching specified selector", name.Name, name.TillerNamespace, c, name.Name)}
			}
		}

		errs := converge(st, helm)

		processed := len(st.Releases) != 0 && len(errs) == 0

		return processed, errs
	})
}

func (a *App) findStateFilesInAbsPaths(specifiedPath string) ([]string, error) {
	rels, err := a.findDesiredStateFiles(specifiedPath)
	if err != nil {
		return rels, err
	}

	files := make([]string, len(rels))
	for i := range rels {
		files[i], err = filepath.Abs(rels[i])
		if err != nil {
			return []string{}, err
		}
	}
	return files, nil
}

func (a *App) findDesiredStateFiles(specifiedPath string) ([]string, error) {
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
	sort.Slice(files, func(i, j int) bool {
		return files[i] < files[j]
	})
	return files, nil
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

func appError(msg string, err error) error {
	return &Error{msg, []error{err}}
}

func (c context) clean(errs []error) error {
	if errs == nil {
		errs = []error{}
	}

	cleanErrs := c.st.Clean()
	if cleanErrs != nil {
		errs = append(errs, cleanErrs...)
	}

	return c.wrapErrs(errs...)
}

type context struct {
	app *App
	st  *state.HelmState
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
