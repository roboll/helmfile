package state

import (
	"bytes"
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/tmpl"
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

	if state.DeprecatedContext != "" && state.HelmDefaults.KubeContext == "" {
		state.HelmDefaults.KubeContext = state.DeprecatedContext
	}

	state.logger = c.logger

	e, err := state.loadEnv(env, c.readFile)
	if err != nil {
		return nil, &StateLoadError{fmt.Sprintf("failed to read %s", file), err}
	}
	state.Env = *e

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

func (st *HelmState) loadEnv(name string, readFile func(string) ([]byte, error)) (*environment.Environment, error) {
	envVals := map[string]interface{}{}
	envSpec, ok := st.Environments[name]
	if ok {
		for _, envvalFile := range envSpec.Values {
			envvalFullPath := filepath.Join(st.basePath, envvalFile)
			tmplData := EnvironmentTemplateData{Environment: environment.EmptyEnvironment, Namespace: ""}
			r := tmpl.NewFileRenderer(readFile, filepath.Dir(envvalFullPath), tmplData)
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
			helm := helmexec.New(st.logger, "")
			for _, secFile := range envSpec.Secrets {
				path := filepath.Join(st.basePath, secFile)
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
