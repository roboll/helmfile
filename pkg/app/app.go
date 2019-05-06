package app

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"

	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"go.uber.org/zap"

	"path/filepath"
	"sort"
	"syscall"
)

type App struct {
	KubeContext string
	Logger      *zap.SugaredLogger
	Reverse     bool
	Env         string
	Namespace   string
	Selectors   []string

	readFile          func(string) ([]byte, error)
	glob              func(string) ([]string, error)
	abs               func(string) (string, error)
	fileExistsAt      func(string) bool
	directoryExistsAt func(string) bool

	getwd func() (string, error)
	chdir func(string) error
}

func Init(app *App) *App {
	app.readFile = ioutil.ReadFile
	app.glob = filepath.Glob
	app.abs = filepath.Abs
	app.getwd = os.Getwd
	app.chdir = os.Chdir
	app.fileExistsAt = fileExistsAt
	app.directoryExistsAt = directoryExistsAt
	return app
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

func (a *App) visitStateFiles(fileOrDir string, do func(string) error) error {
	desiredStateFiles, err := a.findDesiredStateFiles(fileOrDir)
	if err != nil {
		return err
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
			return do(file)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *App) VisitDesiredStates(fileOrDir string, selector []string, converge func(*state.HelmState, helmexec.Interface) (bool, []error)) error {
	noMatchInHelmfiles := true

	err := a.visitStateFiles(fileOrDir, func(f string) error {
		content, err := a.readFile(f)
		if err != nil {
			return err
		}
		// render template, in two runs
		r := &twoPassRenderer{
			reader:    a.readFile,
			env:       a.Env,
			namespace: a.Namespace,
			filename:  f,
			logger:    a.Logger,
			abs:       a.abs,
		}
		yamlBuf, err := r.renderTemplate(content)
		if err != nil {
			return fmt.Errorf("error during %s parsing: %v", f, err)
		}

		st, err := a.loadDesiredStateFromYaml(
			yamlBuf.Bytes(),
			f,
			a.Namespace,
			a.Env,
		)

		helm := helmexec.New(a.Logger, a.KubeContext)

		if err != nil {
			switch stateLoadErr := err.(type) {
			// Addresses https://github.com/roboll/helmfile/issues/279
			case *state.StateLoadError:
				switch stateLoadErr.Cause.(type) {
				case *state.UndefinedEnvError:
					return nil
				default:
					return err
				}
			default:
				return err
			}
		}
		st.Selectors = selector

		if len(st.Helmfiles) > 0 {
			noMatchInSubHelmfiles := true
			for _, m := range st.Helmfiles {
				//assign parent selector to sub helm selector in legacy mode or do not inherit in experimental mode
				if (m.Selectors == nil && !isExplicitSelectorInheritanceEnabled()) || m.SelectorsInherited {
					m.Selectors = selector
				}
				if err := a.VisitDesiredStates(m.Path, m.Selectors, converge); err != nil {
					switch err.(type) {
					case *NoMatchingHelmfileError:

					default:
						return fmt.Errorf("failed processing %s: %v", m.Path, err)
					}
				} else {
					noMatchInSubHelmfiles = false
				}
			}
			noMatchInHelmfiles = noMatchInHelmfiles && noMatchInSubHelmfiles
		}

		templated, tmplErr := st.ExecuteTemplates()
		if tmplErr != nil {
			return fmt.Errorf("failed executing release templates in \"%s\": %v", f, tmplErr)
		}
		processed, errs := converge(templated, helm)
		noMatchInHelmfiles = noMatchInHelmfiles && !processed
		return clean(templated, errs)
	})

	if err != nil {
		return err
	}

	if noMatchInHelmfiles {
		return &NoMatchingHelmfileError{selectors: a.Selectors, env: a.Env}
	}

	return nil
}

func (a *App) VisitDesiredStatesWithReleasesFiltered(fileOrDir string, converge func(*state.HelmState, helmexec.Interface) []error) error {

	err := a.VisitDesiredStates(fileOrDir, a.Selectors, func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
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
	if err != nil {
		return err
	}
	return nil
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
			return []string{}, fmt.Errorf("no state file found. It must be named %s/*.yaml, %s, or %s, or otherwise specified with the --file flag", DefaultHelmfileDirectory, DefaultHelmfile, DeprecatedHelmfile)
		}
	}

	files, err := a.glob(filepath.Join(helmfileDir, "*.yaml"))
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

func directoryExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsDir()
}

func (a *App) loadDesiredStateFromYaml(yaml []byte, file string, namespace string, env string) (*state.HelmState, error) {
	c := state.NewCreator(a.Logger, a.readFile, a.abs)
	st, err := c.CreateFromYaml(yaml, file, env)
	if err != nil {
		return nil, err
	}

	helmfiles := []state.SubHelmfileSpec{}
	for _, hf := range st.Helmfiles {
		globPattern := hf.Path
		var absPathPattern string
		if filepath.IsAbs(globPattern) {
			absPathPattern = globPattern
		} else {
			absPathPattern = st.JoinBase(globPattern)
		}
		matches, err := a.glob(absPathPattern)
		if err != nil {
			return nil, fmt.Errorf("failed processing %s: %v", globPattern, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			newHelmfile := hf
			newHelmfile.Path = match
			helmfiles = append(helmfiles, newHelmfile)
		}

	}
	st.Helmfiles = helmfiles

	if a.Reverse {
		rev := func(i, j int) bool {
			return j < i
		}
		sort.Slice(st.Releases, rev)
		sort.Slice(st.Helmfiles, rev)
	}

	if a.KubeContext != "" {
		if st.HelmDefaults.KubeContext != "" {
			log.Printf("err: Cannot use option --kube-context and set attribute helmDefaults.kubeContext.")
			os.Exit(1)
		}
		st.HelmDefaults.KubeContext = a.KubeContext
	}
	if namespace != "" {
		if st.Namespace != "" {
			log.Printf("err: Cannot use option --namespace and set attribute namespace.")
			os.Exit(1)
		}
		st.Namespace = namespace
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		errs := []error{fmt.Errorf("Received [%s] to shutdown ", sig)}
		_ = clean(st, errs)
		// See http://tldp.org/LDP/abs/html/exitcodes.html
		switch sig {
		case syscall.SIGINT:
			os.Exit(130)
		case syscall.SIGTERM:
			os.Exit(143)
		}
	}()

	return st, nil
}

func clean(st *state.HelmState, errs []error) error {
	if errs == nil {
		errs = []error{}
	}

	cleanErrs := st.Clean()
	if cleanErrs != nil {
		errs = append(errs, cleanErrs...)
	}

	if errs != nil && len(errs) > 0 {
		for _, err := range errs {
			switch e := err.(type) {
			case *state.ReleaseError:
				fmt.Printf("err: release \"%s\" in \"%s\" failed: %v\n", e.Name, st.FilePath, e)
			default:
				fmt.Printf("err: %v\n", e)
			}
		}
		return errs[0]
	}
	return nil
}
