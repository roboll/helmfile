package state

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/roboll/helmfile/pkg/helmexec"
	"io"
	"os"

	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/pkg/environment"
	"github.com/roboll/helmfile/pkg/maputil"
	"github.com/variantdev/vals"
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
	logger      *zap.SugaredLogger
	readFile    func(string) ([]byte, error)
	fileExists  func(string) (bool, error)
	abs         func(string) (string, error)
	glob        func(string) ([]string, error)
	valsRuntime vals.Evaluator

	Strict bool

	LoadFile func(inheritedEnv *environment.Environment, baseDir, file string, evaluateBases bool) (*HelmState, error)

	getHelm func(*HelmState) helmexec.Interface
}

func NewCreator(logger *zap.SugaredLogger, readFile func(string) ([]byte, error), fileExists func(string) (bool, error), abs func(string) (string, error), glob func(string) ([]string, error), valsRuntime vals.Evaluator, getHelm func(*HelmState) helmexec.Interface) *StateCreator {
	return &StateCreator{
		logger:      logger,
		readFile:    readFile,
		fileExists:  fileExists,
		abs:         abs,
		glob:        glob,
		Strict:      true,
		valsRuntime: valsRuntime,
		getHelm:     getHelm,
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
	state.valsRuntime = c.valsRuntime

	return &state, nil
}

// LoadEnvValues loads environment values files relative to the `baseDir`
func (c *StateCreator) LoadEnvValues(target *HelmState, env string, ctxEnv *environment.Environment, failOnMissingEnv bool) (*HelmState, error) {
	state := *target

	e, err := c.loadEnvValues(&state, env, failOnMissingEnv, ctxEnv, c.readFile, c.glob)
	if err != nil {
		return nil, &StateLoadError{fmt.Sprintf("failed to read %s", state.FilePath), err}
	}

	e.Defaults, err = state.loadValuesEntries(nil, state.DefaultValues)
	if err != nil {
		return nil, err
	}

	state.Env = *e

	return &state, nil
}

// Parses YAML into HelmState, while loading environment values files relative to the `baseDir`
// evaluateBases=true means that this is NOT a base helmfile
func (c *StateCreator) ParseAndLoad(content []byte, baseDir, file string, envName string, evaluateBases bool, envValues *environment.Environment) (*HelmState, error) {
	state, err := c.Parse(content, baseDir, file)
	if err != nil {
		return nil, err
	}

	if !evaluateBases {
		if len(state.Bases) > 0 {
			return nil, errors.New("nested `base` helmfile is unsupported. please submit a feature request if you need this!")
		}
	} else {
		state, err = c.loadBases(envValues, state, baseDir)
		if err != nil {
			return nil, err
		}
	}

	state, err = c.LoadEnvValues(state, envName, envValues, evaluateBases)
	if err != nil {
		return nil, err
	}

	state.FilePath = file

	return state, nil
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

func (c *StateCreator) loadEnvValues(st *HelmState, name string, failOnMissingEnv bool, ctxEnv *environment.Environment, readFile func(string) ([]byte, error), glob func(string) ([]string, error)) (*environment.Environment, error) {
	envVals := map[string]interface{}{}
	envSpec, ok := st.Environments[name]
	if ok {
		var err error
		envVals, err = st.loadValuesEntries(envSpec.MissingFileHandler, envSpec.Values)
		if err != nil {
			return nil, err
		}

		if len(envSpec.Secrets) > 0 {

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
			if err = c.scatterGatherEnvSecretFiles(st, envSecretFiles, envVals, readFile); err != nil {
				return nil, err
			}
		}
	} else if ctxEnv == nil && name != DefaultEnv && failOnMissingEnv {
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

func (c *StateCreator) scatterGatherEnvSecretFiles(st *HelmState, envSecretFiles []string, envVals map[string]interface{}, readFile func(string) ([]byte, error)) error {
	var errs []error

	inputs := envSecretFiles
	inputsSize := len(inputs)

	type secretResult struct {
		result map[string]interface{}
		err    error
		path   string
	}

	secrets := make(chan string, inputsSize)
	results := make(chan secretResult, inputsSize)

	st.scatterGather(0, inputsSize,
		func() {
			for _, secretFile := range envSecretFiles {
				secrets <- secretFile
			}
			close(secrets)
		},
		func(id int) {
			for path := range secrets {
				release := &ReleaseSpec{}
				flags := st.appendConnectionFlags([]string{}, release)
				decFile, err := c.getHelm(st).DecryptSecret(st.createHelmContext(release, 0), path, flags...)
				if err != nil {
					results <- secretResult{nil, err, path}
					continue
				}
				bytes, err := readFile(decFile)
				if err != nil {
					results <- secretResult{nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", path, err), path}
					continue
				}
				m := map[string]interface{}{}
				if err := yaml.Unmarshal(bytes, &m); err != nil {
					results <- secretResult{nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", path, err), path}
					continue
				}
				// All the nested map key should be string. Otherwise we get strange errors due to that
				// mergo or reflect is unable to merge map[interface{}]interface{} with map[string]interface{} or vice versa.
				// See https://github.com/roboll/helmfile/issues/677
				vals, err := maputil.CastKeysToStrings(m)
				if err != nil {
					results <- secretResult{nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", path, err), path}
					continue
				}
				results <- secretResult{vals, nil, path}
			}
		},
		func() {
			for i := 0; i < inputsSize; i++ {
				result := <-results
				if result.err != nil {
					errs = append(errs, result.err)
				} else {
					if err := mergo.Merge(&envVals, &result.result, mergo.WithOverride); err != nil {
						errs = append(errs, fmt.Errorf("failed to load environment secrets file \"%s\": %v", result.path, err))
					}
				}
			}
			close(results)
		},
	)

	if len(errs) > 1 {
		for _, err := range errs {
			st.logger.Error(err)
		}
		return fmt.Errorf("Failed loading environment secrets with %d errors", len(errs))
	}
	return nil
}

func (st *HelmState) loadValuesEntries(missingFileHandler *string, entries []interface{}) (map[string]interface{}, error) {
	envVals := map[string]interface{}{}

	valuesEntries := append([]interface{}{}, entries...)
	ld := NewEnvironmentValuesLoader(st.storage(), st.readFile, st.logger)
	var err error
	envVals, err = ld.LoadEnvironmentValues(missingFileHandler, valuesEntries)
	if err != nil {
		return nil, err
	}

	return envVals, nil
}
