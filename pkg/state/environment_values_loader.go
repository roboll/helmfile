package state

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/pkg/environment"
	"github.com/roboll/helmfile/pkg/maputil"
	"github.com/roboll/helmfile/pkg/tmpl"
	"gopkg.in/yaml.v2"
	"path/filepath"
)

type EnvironmentValuesLoader struct {
	storage *Storage

	readFile func(string) ([]byte, error)
}

func NewEnvironmentValuesLoader(storage *Storage, readFile func(string) ([]byte, error)) *EnvironmentValuesLoader {
	return &EnvironmentValuesLoader{
		storage:  storage,
		readFile: readFile,
	}
}

func (ld *EnvironmentValuesLoader) LoadEnvironmentValues(missingFileHandler *string, envValues []interface{}) (map[string]interface{}, error) {
	envVals := map[string]interface{}{}

	for _, v := range envValues {
		switch typedValue := v.(type) {
		case string:
			urlOrPath := typedValue
			resolved, skipped, err := ld.storage.resolveFile(missingFileHandler, "environment values", urlOrPath)
			if err != nil {
				return nil, err
			}
			if skipped {
				continue
			}

			for _, envvalFullPath := range resolved {
				tmplData := EnvironmentTemplateData{Environment: environment.EmptyEnvironment, Namespace: ""}
				r := tmpl.NewFileRenderer(ld.readFile, filepath.Dir(envvalFullPath), tmplData)
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
		case map[interface{}]interface{}:
			m, err := maputil.CastKeysToStrings(typedValue)
			if err != nil {
				return nil, err
			}
			if err := mergo.Merge(&envVals, &m, mergo.WithOverride); err != nil {
				return nil, fmt.Errorf("failed to merge %v: %v", typedValue, err)
			}
			continue
		default:
			return nil, fmt.Errorf("unexpected type of value: value=%v, type=%T", typedValue, typedValue)
		}
	}

	return envVals, nil
}
