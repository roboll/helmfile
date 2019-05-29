package state

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/helmexec"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"io"
	"os"
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
	logger     *zap.SugaredLogger
	readFile   func(string) ([]byte, error)
	fileExists func(string) (bool, error)
	abs        func(string) (string, error)
	glob       func(string) ([]string, error)

	Strict bool

	LoadFile func(inheritedEnv *environment.Environment, baseDir, file string, evaluateBases bool) (*HelmState, error)
}

func NewCreator(logger *zap.SugaredLogger, readFile func(string) ([]byte, error), fileExists func(string) (bool, error), abs func(string) (string, error), glob func(string) ([]string, error)) *StateCreator {
	return &StateCreator{
		logger:     logger,
		readFile:   readFile,
		fileExists: fileExists,
		abs:        abs,
		glob:       glob,
		Strict:     true,
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
	state.fileExists = c.fileExists
	state.glob = c.glob

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

	state, err = c.loadBases(envValues, state, baseDir)
	if err != nil {
		return nil, err
	}

	return c.LoadEnvValues(state, envName, envValues)
}

func (c *StateCreator) loadBases(envValues *environment.Environment, st *HelmState, baseDir string) (*HelmState, error) {
	layers := []*HelmState{}
	for _, b := range st.Bases {
		base, err := c.LoadFile(envValues, baseDir, b, false)
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

func (st *HelmState) loadEnvValues(name string, ctxEnv *environment.Environment, readFile func(string) ([]byte, error), glob func(string) ([]string, error)) (*environment.Environment, error) {
	envVals := map[string]interface{}{}
	envSpec, ok := st.Environments[name]
	if ok {
		envValues := append([]interface{}{}, envSpec.Values...)
		ld := &EnvironmentValuesLoader{
			storage:  st.storage(),
			readFile: st.readFile,
		}
		var err error
		envVals, err = ld.LoadEnvironmentValues(envSpec.MissingFileHandler, envValues)
		if err != nil {
			return nil, err
		}

		if len(envSpec.Secrets) > 0 {
			helm := helmexec.New(st.logger, "")

			var envSecretFiles []string
			for _, urlOrPath := range envSpec.Secrets {
				resolved, skipped, err := st.storage().resolveFile(envSpec.MissingFileHandler, "environment values", urlOrPath)
				if err != nil {
					return nil, err
				}
				if skipped {
					continue
				}

				envSecretFiles = append(envSecretFiles, resolved...)
			}

			for _, path := range envSecretFiles {
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

		if err := mergo.Merge(&intEnv, newEnv, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("error while merging environment values for \"%s\": %v", name, err)
		}

		newEnv = &intEnv
	}

	return newEnv, nil
}
