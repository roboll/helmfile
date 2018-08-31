package state

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/valuesfile"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
)

func CreateFromYaml(content []byte, file string, env string, logger *zap.SugaredLogger) (*HelmState, error) {
	return createFromYamlWithFileReader(content, file, env, logger, ioutil.ReadFile)
}

func createFromYamlWithFileReader(content []byte, file string, env string, logger *zap.SugaredLogger, readFile func(string) ([]byte, error)) (*HelmState, error) {
	var state HelmState

	state.basePath, _ = filepath.Abs(filepath.Dir(file))
	if err := yaml.UnmarshalStrict(content, &state); err != nil {
		return nil, err
	}
	state.FilePath = file

	if len(state.DeprecatedReleases) > 0 {
		if len(state.Releases) > 0 {
			return nil, fmt.Errorf("failed to parse %s: you can't specify both `charts` and `releases` sections", file)
		}
		state.Releases = state.DeprecatedReleases
		state.DeprecatedReleases = []ReleaseSpec{}
	}

	state.logger = logger

	e, err := state.loadEnv(env, readFile)
	if err != nil {
		return nil, err
	}
	state.env = *e

	state.readFile = readFile

	return &state, nil
}

func (state *HelmState) loadEnv(name string, readFile func(string) ([]byte, error)) (*environment.Environment, error) {
	envVals := map[string]interface{}{}
	envSpec, ok := state.Environments[name]
	if ok {
		r := valuesfile.NewRenderer(readFile, state.basePath, environment.EmptyEnvironment)
		for _, envvalFile := range envSpec.Values {
			bytes, err := r.RenderToBytes(filepath.Join(state.basePath, envvalFile))
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
	} else if name != DefaultEnv {
		return nil, fmt.Errorf("environment \"%s\" is not defined in \"%s\"", name, state.FilePath)
	}

	return &environment.Environment{Name: name, Values: envVals}, nil
}
