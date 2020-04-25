package tmpl

import (
	"errors"
	"fmt"
	"github.com/roboll/helmfile/pkg/plugins"
	"github.com/variantdev/vals"
	"sync"
)

//to generate mock run mockgen -source=expand_secret_ref.go -destination=expand_secrets_mock.go -package=tmpl
type valClient interface {
	Eval(template map[string]interface{}) (map[string]interface{}, error)
}

var once sync.Once
var secretsClient valClient

func fetchSecretValue(path string) (string, error) {
	tmpMap := make(map[string]interface{})
	tmpMap["key"] = path
	resultMap, err := fetchSecretValues(tmpMap)
	if err != nil {
		return "", err
	}

	rendered, ok := resultMap["key"]
	if !ok {
		return "", errors.New(fmt.Sprintf("unexpected error occurred, %v doesn't have 'key' key", resultMap))
	}

	result, ok := rendered.(string)
	if !ok {
		return "", errors.New(fmt.Sprintf("expected %v to be string", rendered))
	}

	return result, nil
}

func fetchSecretValues(values map[string]interface{}) (map[string]interface{}, error) {
	var err error
	// below lines are for tests
	once.Do(func() {
		var valRuntime *vals.Runtime
		if secretsClient == nil {
			valRuntime, err = plugins.ValsInstance()
			if err != nil {
				return
			}
			secretsClient = valRuntime
		}
	})
	if secretsClient == nil {
		return nil, err
	}

	resultMap, err := secretsClient.Eval(values)
	if err != nil {
		return nil, err
	}

	return resultMap, nil
}
