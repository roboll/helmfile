package app

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/state"
	"go.uber.org/zap"
	"path/filepath"
	"sort"
)

type desiredStateLoader struct {
	KubeContext string
	Reverse     bool

	env       string
	namespace string

	readFile   func(string) ([]byte, error)
	fileExists func(string) (bool, error)
	abs        func(string) (string, error)
	glob       func(string) ([]string, error)

	logger *zap.SugaredLogger
}

type LoadOpts struct {
	Selectors   []string
	Environment state.SubhelmfileEnvironmentSpec
	CalleePath  string
}

func (ld *desiredStateLoader) Load(f string, opts LoadOpts) (*state.HelmState, error) {
	var inheritedEnv *environment.Environment

	args := opts.Environment.AdditionalValues

	if len(args) > 0 {
		if opts.CalleePath == "" {
			return nil, fmt.Errorf("bug: opts.CalleePath was nil: f=%s, opts=%v", f, opts)
		}
		storage := state.NewStorage(opts.CalleePath, ld.logger, ld.glob)
		envld := state.NewEnvironmentValuesLoader(storage, ld.readFile)
		handler := state.MissingFileHandlerError
		vals, err := envld.LoadEnvironmentValues(&handler, args)
		if err != nil {
			return nil, err
		}

		inheritedEnv = &environment.Environment{
			Name:   ld.env,
			Values: vals,
		}
	}

	st, err := ld.loadFile(inheritedEnv, filepath.Dir(f), filepath.Base(f), true)
	if err != nil {
		return nil, err
	}

	if ld.Reverse {
		rev := func(i, j int) bool {
			return j < i
		}
		sort.Slice(st.Releases, rev)
		sort.Slice(st.Helmfiles, rev)
	}

	if ld.KubeContext != "" {
		if st.HelmDefaults.KubeContext != "" {
			return nil, errors.New("err: Cannot use option --kube-context and set attribute helmDefaults.kubeContext.")
		}
		st.HelmDefaults.KubeContext = ld.KubeContext
	}

	if ld.namespace != "" {
		if st.Namespace != "" {
			return nil, errors.New("err: Cannot use option --namespace and set attribute namespace.")
		}
		st.Namespace = ld.namespace
	}

	return st, nil
}

func (ld *desiredStateLoader) loadFile(inheritedEnv *environment.Environment, baseDir, file string, evaluateBases bool) (*state.HelmState, error) {
	var f string
	if filepath.IsAbs(file) {
		f = file
	} else {
		f = filepath.Join(baseDir, file)
	}

	fileBytes, err := ld.readFile(f)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(f)

	var self *state.HelmState

	if !experimentalModeEnabled() || ext == ".gotmpl" {
		self, err = ld.renderAndLoad(
			inheritedEnv,
			baseDir,
			f,
			fileBytes,
			evaluateBases,
		)
	} else {
		self, err = ld.load(
			fileBytes,
			baseDir,
			file,
			evaluateBases,
			inheritedEnv,
		)
	}

	return self, err
}

func (a *desiredStateLoader) underlying() *state.StateCreator {
	c := state.NewCreator(a.logger, a.readFile, a.fileExists, a.abs, a.glob)
	c.LoadFile = a.loadFile
	return c
}

func (a *desiredStateLoader) load(yaml []byte, baseDir, file string, evaluateBases bool, env *environment.Environment) (*state.HelmState, error) {
	st, err := a.underlying().ParseAndLoad(yaml, baseDir, file, a.env, evaluateBases, env)
	if err != nil {
		return nil, err
	}

	helmfiles, err := st.ExpandedHelmfiles()
	if err != nil {
		return nil, err
	}
	st.Helmfiles = helmfiles

	return st, nil
}

func (ld *desiredStateLoader) renderAndLoad(env *environment.Environment, baseDir, filename string, content []byte, evaluateBases bool) (*state.HelmState, error) {
	parts := bytes.Split(content, []byte("\n---\n"))

	var finalState *state.HelmState

	for i, part := range parts {
		var yamlBuf *bytes.Buffer
		var err error

		id := fmt.Sprintf("%s.part.%d", filename, i)

		if env == nil {
			yamlBuf, err = ld.renderTemplatesToYaml(baseDir, id, part)
			if err != nil {
				return nil, fmt.Errorf("error during %s parsing: %v", id, err)
			}
		} else {
			yamlBuf, err = ld.renderTemplatesToYaml(baseDir, id, part, *env)
			if err != nil {
				return nil, fmt.Errorf("error during %s parsing: %v", id, err)
			}
		}

		currentState, err := ld.load(
			yamlBuf.Bytes(),
			baseDir,
			filename,
			evaluateBases,
			env,
		)
		if err != nil {
			return nil, err
		}

		if finalState == nil {
			finalState = currentState
		} else {
			if err := mergo.Merge(finalState, currentState, mergo.WithAppendSlice); err != nil {
				return nil, err
			}
		}

		env = &finalState.Env

		ld.logger.Debugf("merged environment: %v", env)
	}

	return finalState, nil
}
