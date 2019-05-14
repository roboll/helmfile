package app

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/state"
	"go.uber.org/zap"
	"log"
	"os"
	"path/filepath"
	"sort"
)

type desiredStateLoader struct {
	KubeContext string
	Reverse     bool

	env       string
	namespace string

	readFile func(string) ([]byte, error)
	abs      func(string) (string, error)
	glob     func(string) ([]string, error)

	logger *zap.SugaredLogger
}

func (ld *desiredStateLoader) Load(f string) (*state.HelmState, error) {
	return ld.loadFile(filepath.Dir(f), filepath.Base(f), true)
}

func (ld *desiredStateLoader) loadFile(baseDir, file string, evaluateBases bool) (*state.HelmState, error) {
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
		)
	}

	return self, err
}

func (a *desiredStateLoader) load(yaml []byte, baseDir, file string, evaluateBases bool) (*state.HelmState, error) {
	c := state.NewCreator(a.logger, a.readFile, a.abs)
	st, err := c.ParseAndLoadEnv(yaml, baseDir, file, a.env)
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
	if a.namespace != "" {
		if st.Namespace != "" {
			log.Printf("err: Cannot use option --namespace and set attribute namespace.")
			os.Exit(1)
		}
		st.Namespace = a.namespace
	}

	if err != nil {
		return nil, err
	}

	if !evaluateBases {
		if len(st.Bases) > 0 {
			return nil, errors.New("nested `base` helmfile is unsupported. please submit a feature request if you need this!")
		}

		return st, nil
	}

	layers := []*state.HelmState{}
	for _, b := range st.Bases {
		base, err := a.loadFile(baseDir, b, false)
		if err != nil {
			return nil, err
		}
		layers = append(layers, base)
	}
	layers = append(layers, st)

	for i := 1; i < len(layers); i++ {
		if err := mergo.Merge(layers[0], layers[i], mergo.WithAppendSlice); err != nil {
			return nil, err
		}
	}

	return layers[0], nil
}

func (ld *desiredStateLoader) renderAndLoad(baseDir, filename string, content []byte, evaluateBases bool) (*state.HelmState, error) {
	parts := bytes.Split(content, []byte("\n---\n"))

	var finalState *state.HelmState
	var env *environment.Environment

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
