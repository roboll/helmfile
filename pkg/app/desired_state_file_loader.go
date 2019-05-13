package app

import (
	"fmt"
	"github.com/imdario/mergo"
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
	return ld.load(filepath.Dir(f), filepath.Base(f), true)
}

func (ld *desiredStateLoader) load(baseDir, file string, evaluateBases bool) (*state.HelmState, error) {
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

	var yamlBytes []byte
	if !experimentalModeEnabled() || ext == ".gotmpl" {
		yamlBuf, err := ld.renderTemplateToYaml(baseDir, f, fileBytes)
		if err != nil {
			return nil, fmt.Errorf("error during %s parsing: %v", f, err)
		}
		yamlBytes = yamlBuf.Bytes()
	} else {
		yamlBytes = fileBytes
	}

	self, err := ld.loadYaml(
		yamlBytes,
		baseDir,
		file,
	)

	if err != nil {
		return nil, err
	}

	if !evaluateBases {
		return self, nil
	}

	layers := []*state.HelmState{}
	for _, b := range self.Bases {
		base, err := ld.load(baseDir, b, false)
		if err != nil {
			return nil, err
		}
		layers = append(layers, base)
	}
	layers = append(layers, self)

	for i := 1; i < len(layers); i++ {
		if err := mergo.Merge(layers[0], layers[i], mergo.WithAppendSlice); err != nil {
			return nil, err
		}
	}

	return layers[0], nil
}

func (a *desiredStateLoader) loadYaml(yaml []byte, baseDir, file string) (*state.HelmState, error) {
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

	return st, nil
}
