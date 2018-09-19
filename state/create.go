package state

import (
	"bytes"
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/valuesfile"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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

func createFromYaml(content []byte, file string, env string, logger *zap.SugaredLogger) (*HelmState, error) {
	c := &creator{
		logger,
		ioutil.ReadFile,
		filepath.Abs,
		true,
	}
	return c.CreateFromYaml(content, file, env)
}

type creator struct {
	logger   *zap.SugaredLogger
	readFile func(string) ([]byte, error)
	abs      func(string) (string, error)

	Strict bool
}

func NewCreator(logger *zap.SugaredLogger, readFile func(string) ([]byte, error), abs func(string) (string, error)) *creator {
	return &creator{
		logger:   logger,
		readFile: readFile,
		abs:      abs,
		Strict:   true,
	}
}

func (c *creator) CreateFromYaml(content []byte, file string, env string) (*HelmState, error) {
	var state HelmState

	basePath, err := c.abs(filepath.Dir(file))
	if err != nil {
		return nil, &StateLoadError{fmt.Sprintf("failed to read %s", file), err}
	}
	state.FilePath = file
	state.basePath = basePath

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

	state.logger = c.logger

	e, err := state.loadEnv(env, c.readFile)
	if err != nil {
		return nil, &StateLoadError{fmt.Sprintf("failed to read %s", file), err}
	}
	state.Env = *e

	state.readFile = c.readFile

	return &state, nil
}

func (state *HelmState) loadEnv(name string, readFile func(string) ([]byte, error)) (*environment.Environment, error) {
	envVals := map[string]interface{}{}
	envSpec, ok := state.Environments[name]
	if ok {
		for _, envvalFile := range envSpec.Values {
			envvalFullPath := filepath.Join(state.basePath, envvalFile)
			r := valuesfile.NewRenderer(readFile, filepath.Dir(envvalFullPath), environment.EmptyEnvironment)
			bytes, err := r.RenderToBytes(envvalFullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load environment values file \"%s\": %v", envvalFile, err)
			}
			m := map[string]interface{}{}
			if err := yaml.Unmarshal(bytes, &m); err != nil {
				return nil, fmt.Errorf("failed to load environment values file \"%s\": %v", envvalFile, err)
			}
			if err := mergo.Merge(&envVals, &m, mergo.WithOverride); err != nil {
				return nil, fmt.Errorf("failed to load \"%s\": %v", envvalFile, err)
			}
		}

		if len(envSpec.Secrets) > 0 {
			helm := helmexec.New(state.logger, "")
			for _, secFile := range envSpec.Secrets {
				path := filepath.Join(state.basePath, secFile)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					return nil, err
				}

				decFile, err := helm.DecryptSecret(path)
				if err != nil {
					return nil, err
				}
				bytes, err := readFile(decFile)
				if err != nil {
					return nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", secFile, err)
				}
				m := map[string]interface{}{}
				if err := yaml.Unmarshal(bytes, &m); err != nil {
					return nil, fmt.Errorf("failed to load environment secrets file \"%s\": %v", secFile, err)
				}
				if err := mergo.Merge(&envVals, &m, mergo.WithOverride); err != nil {
					return nil, fmt.Errorf("failed to load \"%s\": %v", secFile, err)
				}
			}
		}
	} else if name != DefaultEnv {
		return nil, &UndefinedEnvError{msg: fmt.Sprintf("environment \"%s\" is not defined", name)}
	}

	return &environment.Environment{Name: name, Values: envVals}, nil
}
