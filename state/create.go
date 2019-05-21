package state

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/tmpl"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

type StateLoadError struct {
	msg   string
	Cause error
}

func (e *StateLoadError) Error() string {
	return fmt.Sprintf("%s: %v", e.msg, e.Cause)
}

type UndefinedEnvError struct {
	msg string
}

func (e *UndefinedEnvError) Error() string {
	return e.msg
}

type StateCreator struct {
	logger   *zap.SugaredLogger
	readFile func(string) ([]byte, error)
	abs      func(string) (string, error)
	glob     func(string) ([]string, error)

	Strict bool

	LoadFile func(baseDir, file string, evaluateBases bool) (*HelmState, error)
}

func NewCreator(logger *zap.SugaredLogger, readFile func(string) ([]byte, error), abs func(string) (string, error), glob func(string) ([]string, error)) *StateCreator {
	return &StateCreator{
		logger:   logger,
		readFile: readFile,
		abs:      abs,
		glob:     glob,
		Strict:   true,
	}
}

// Parse parses YAML into HelmState
func (c *StateCreator) Parse(content []byte, baseDir, file string) (*HelmState, error) {
	var state HelmState

	state.FilePath = file
	state.basePath = baseDir

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	if !c.Strict {
		decoder.SetStrict(false)
	} else {
		decoder.SetStrict(true)
	}
	i := 0
	for {
		i++

		var intermediate HelmState

		err := decoder.Decode(&intermediate)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, &StateLoadError{fmt.Sprintf("failed to read %s: reading document at index %d", file, i), err}
		}

		if err := mergo.Merge(&state, &intermediate, mergo.WithAppendSlice); err != nil {
			return nil, &StateLoadError{fmt.Sprintf("failed to read %s: merging document at index %d", file, i), err}
		}
	}

	if len(state.DeprecatedReleases) > 0 {
		if len(state.Releases) > 0 {
			return nil, fmt.Errorf("failed to parse %s: you can't specify both `charts` and `releases` sections", file)
		}
		state.Releases = state.DeprecatedReleases
		state.DeprecatedReleases = []ReleaseSpec{}
	}

	if state.DeprecatedContext != "" && state.HelmDefaults.KubeContext == "" {
		state.HelmDefaults.KubeContext = state.DeprecatedContext
	}

	state.logger = c.logger

	state.readFile = c.readFile
	state.removeFile = os.Remove
	state.fileExists = func(path string) (bool, error) {
		_, err := os.Stat(path)

		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	return &state, nil
}

// LoadEnvValues loads environment values files relative to the `baseDir`
func (c *StateCreator) LoadEnvValues(target *HelmState, env string, ctxEnv *environment.Environment) (*HelmState, error) {
	state := *target

	e, err := state.loadEnvValues(env, ctxEnv, c.readFile, c.glob)
	if err != nil {
		return nil, &StateLoadError{fmt.Sprintf("failed to read %s", state.FilePath), err}
	}
	state.Env = *e

	return &state, nil
}

// Parses YAML into HelmState, while loading environment values files relative to the `baseDir`
func (c *StateCreator) ParseAndLoad(content []byte, baseDir, file string, envName string, evaluateBases bool, envValues *environment.Environment) (*HelmState, error) {
	state, err := c.Parse(content, baseDir, file)
	if err != nil {
		return nil, err
	}

	if !evaluateBases {
		if len(state.Bases) > 0 {
			return nil, errors.New("nested `base` helmfile is unsupported. please submit a feature request if you need this!")
		}
	}

	state, err = c.loadBases(state, baseDir)
	if err != nil {
		return nil, err
	}

	return c.LoadEnvValues(state, envName, envValues)
}

func (c *StateCreator) loadBases(st *HelmState, baseDir string) (*HelmState, error) {
	layers := []*HelmState{}
	for _, b := range st.Bases {
		base, err := c.LoadFile(baseDir, b, false)
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

func (st *HelmState) ExpandPaths(patterns []string, glob func(string) ([]string, error)) ([]string, error) {
	result := []string{}
	for _, globPattern := range patterns {
		var absPathPattern string
		if filepath.IsAbs(globPattern) {
			absPathPattern = globPattern
		} else {
			absPathPattern = st.JoinBase(globPattern)
		}
		matches, err := glob(absPathPattern)
		if err != nil {
			return nil, fmt.Errorf("failed processing %s: %v", globPattern, err)
		}

		if len(matches) == 0 {
			return nil, fmt.Errorf("no file matching %s found", globPattern)
		}

		sort.Strings(matches)

		result = append(result, matches...)
	}
	return result, nil
}

func (st *HelmState) loadEnvValues(name string, ctxEnv *environment.Environment, readFile func(string) ([]byte, error), glob func(string) ([]string, error)) (*environment.Environment, error) {
	envVals := map[string]interface{}{}
	envSpec, ok := st.Environments[name]
	if ok {
		valuesFiles, err := st.ExpandPaths(envSpec.Values, glob)
		if err != nil {
			return nil, err
		}

		for _, envvalFullPath := range valuesFiles {
			tmplData := EnvironmentTemplateData{Environment: environment.EmptyEnvironment, Namespace: ""}
			r := tmpl.NewFileRenderer(readFile, filepath.Dir(envvalFullPath), tmplData)
			bytes, err := r.RenderToBytes(envvalFullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load environment values file \"%s\": %v", envvalFullPath, err)
			}
			m := map[string]interface{}{}
			if err := yaml.Unmarshal(bytes, &m); err != nil {
				return nil, fmt.Errorf("failed to load environment values file \"%s\": %v", envvalFullPath, err)
			}
			if err := mergo.Merge(&envVals, &m, mergo.WithOverride); err != nil {
				return nil, fmt.Errorf("failed to load \"%s\": %v", envvalFullPath, err)
			}
		}

		if len(envSpec.Secrets) > 0 {
			secretsFiles, err := st.ExpandPaths(envSpec.Secrets, glob)
			if err != nil {
				return nil, err
			}

			helm := helmexec.New(st.logger, "")
			for _, path := range secretsFiles {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					return nil, err
				}
				// Work-around to allow decrypting environment secrets
				//
				// We don't have releases loaded yet and therefore unable to decide whether
				// helmfile should use helm-tiller to call helm-secrets or not.
				//
				// This means that, when you use environment secrets + tillerless setup, you still need a tiller
				// installed on the cluster, just for decrypting secrets!
				// Related: https://github.com/futuresimple/helm-secrets/issues/83
				release := &ReleaseSpec{}
				flags := st.appendTillerFlags([]string{}, release)
				decFile, err := helm.DecryptSecret(st.createHelmContext(release, 0), path, flags...)
				if err != nil {
					return nil, err
				}
				bytes, err := readFile(decFile)
				if err != nil {
					return nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", path, err)
				}
				m := map[string]interface{}{}
				if err := yaml.Unmarshal(bytes, &m); err != nil {
					return nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", path, err)
				}
				if err := mergo.Merge(&envVals, &m, mergo.WithOverride); err != nil {
					return nil, fmt.Errorf("failed to load \"%s\": %v", path, err)
				}
			}
		}
	} else if ctxEnv == nil && name != DefaultEnv {
		return nil, &UndefinedEnvError{msg: fmt.Sprintf("environment \"%s\" is not defined", name)}
	}

	newEnv := &environment.Environment{Name: name, Values: envVals}

	if ctxEnv != nil {
		intEnv := *ctxEnv

		if err := mergo.Merge(&intEnv, newEnv, mergo.WithAppendSlice); err != nil {
			return nil, fmt.Errorf("error while merging environment values for \"%s\": %v", name, err)
		}

		newEnv = &intEnv
	}

	return newEnv, nil
}
