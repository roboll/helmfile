package state

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/pkg/environment"
	"github.com/roboll/helmfile/pkg/maputil"
	"github.com/roboll/helmfile/pkg/remote"
	"github.com/roboll/helmfile/pkg/tmpl"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"path/filepath"
)

type EnvironmentValuesLoader struct {
	storage *Storage

	readFile func(string) ([]byte, error)

	logger *zap.SugaredLogger

	remote *remote.Remote
}

func NewEnvironmentValuesLoader(storage *Storage, readFile func(string) ([]byte, error), logger *zap.SugaredLogger, remote *remote.Remote) *EnvironmentValuesLoader {
	return &EnvironmentValuesLoader{
		storage:  storage,
		readFile: readFile,
		logger:   logger,
		remote:   remote,
	}
}

func (ld *EnvironmentValuesLoader) LoadEnvironmentValues(missingFileHandler *string, valuesEntries []interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	for _, entry := range valuesEntries {
		maps := []interface{}{}

		switch strOrMap := entry.(type) {
		case string:
			urlOrPath := strOrMap
			localPath, err := ld.remote.Locate(urlOrPath)
			if err == nil {
				urlOrPath = localPath
			}

			files, skipped, err := ld.storage.resolveFile(missingFileHandler, "environment values", urlOrPath)
			if err != nil {
				return nil, err
			}
			if skipped {
				continue
			}

			for _, f := range files {
				tmplData := EnvironmentTemplateData{environment.EmptyEnvironment, "", map[string]interface{}{}}
				r := tmpl.NewFileRenderer(ld.readFile, filepath.Dir(f), tmplData)
				bytes, err := r.RenderToBytes(f)
				if err != nil {
					return nil, fmt.Errorf("failed to load environment values file \"%s\": %v", f, err)
				}
				m := map[string]interface{}{}
				if err := yaml.Unmarshal(bytes, &m); err != nil {
					return nil, fmt.Errorf("failed to load environment values file \"%s\": %v\n\nOffending YAML:\n%s", f, err, bytes)
				}
				maps = append(maps, m)
				if ld.logger != nil {
					ld.logger.Debugf("envvals_loader: loaded %s:%v", strOrMap, m)
				}
			}
		case map[interface{}]interface{}:
			maps = append(maps, strOrMap)
		default:
			return nil, fmt.Errorf("unexpected type of value: value=%v, type=%T", strOrMap, strOrMap)
		}
		for _, m := range maps {
			// All the nested map key should be string. Otherwise we get strange errors due to that
			// mergo or reflect is unable to merge map[interface{}]interface{} with map[string]interface{} or vice versa.
			// See https://github.com/roboll/helmfile/issues/677
			vals, err := maputil.CastKeysToStrings(m)
			if err != nil {
				return nil, err
			}
			if err := mergo.Merge(&result, &vals, mergo.WithOverride, mergo.WithOverwriteWithEmptyValue); err != nil {
				return nil, fmt.Errorf("failed to merge %v: %v", m, err)
			}
		}
	}

	return result, nil
}
