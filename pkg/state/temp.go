package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/roboll/helmfile/pkg/runtime"
)

func createTempValuesFile(release *ReleaseSpec, data interface{}) (*os.File, error) {
	p, err := tempValuesFilePath(release, data)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(*p)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func tempValuesFilePath(release *ReleaseSpec, data interface{}) (*string, error) {
	id, err := generateValuesID(release, data)
	if err != nil {
		panic(err)
	}

	d := filepath.Join(runtime.TempDir(""), id)

	_, err = os.Stat(d)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		panic(err)
	}

	return &d, nil
}

func generateValuesID(release *ReleaseSpec, data interface{}) (string, error) {
	var id []string

	if release.Namespace != "" {
		id = append(id, release.Namespace)
	}

	id = append(id, release.Name, "values")

	hash, err := runtime.HashObject([]interface{}{release, data})
	if err != nil {
		return "", err
	}

	id = append(id, hash)

	return strings.Join(id, "-"), nil
}
