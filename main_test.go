package main

import "testing"

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
	_, _, _, err := loadDesiredStateFromFile(yamlContent, yamlFile, "default", "default", []string{}, "default", logger)
	if err == nil {
		t.Error("error expected but not happened")
	}
	if err.Error() != "duplicate release \"myrelease1\" found: there were 2 releases named \"myrelease1\" matching specified selector" {
		t.Errorf("unexpected error happened: %v", err)
	}
}
