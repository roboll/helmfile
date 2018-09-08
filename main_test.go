package main

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

// See https://github.com/roboll/helmfile/issues/193
func TestReadFromYaml_DuplicateReleaseName(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease1
  chart: mychart1
  labels:
    stage: pre
    foo: bar
- name: myrelease1
  chart: mychart2
  labels:
    stage: post
`)
	app := &app{
		readFile:    ioutil.ReadFile,
		glob:        filepath.Glob,
		abs:         filepath.Abs,
		kubeContext: "default",
		logger:      logger,
	}
	_, _, err := app.loadDesiredStateFromYaml(yamlContent, yamlFile, "default", []string{}, "default")
	if err == nil {
		t.Error("error expected but not happened")
	}
	if err.Error() != "duplicate release \"myrelease1\" found: there were 2 releases named \"myrelease1\" matching specified selector" {
		t.Errorf("unexpected error happened: %v", err)
	}
}
