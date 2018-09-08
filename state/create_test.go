package state

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadFromYaml(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease
  namespace: mynamespace
  chart: mychart
`)
	state, err := createFromYaml(yamlContent, yamlFile, DefaultEnv, logger)
	if err != nil {
		t.Errorf("unxpected error: %v", err)
	}

	if state.Releases[0].Name != "myrelease" {
		t.Errorf("unexpected release name: expected=myrelease actual=%s", state.Releases[0].Name)
	}
	if state.Releases[0].Namespace != "mynamespace" {
		t.Errorf("unexpected chart namespace: expected=mynamespace actual=%s", state.Releases[0].Chart)
	}
	if state.Releases[0].Chart != "mychart" {
		t.Errorf("unexpected chart name: expected=mychart actual=%s", state.Releases[0].Chart)
	}
}

func TestReadFromYaml_InexistentEnv(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease
  namespace: mynamespace
  chart: mychart
`)
	_, err := createFromYaml(yamlContent, yamlFile, "production", logger)
	if err == nil {
		t.Error("expected error")
	}
}

func TestReadFromYaml_NonDefaultEnv(t *testing.T) {
	yamlFile := "/example/path/to/helmfile.yaml"
	yamlContent := []byte(`environments:
  production:
    values:
    - foo.yaml
    - bar.yaml.gotmpl

releases:
- name: myrelease
  namespace: mynamespace
  chart: mychart
  values:
  - values.yaml.gotmpl
`)

	fooYamlFile := "/example/path/to/foo.yaml"
	fooYamlContent := []byte(`foo: foo
# As this file doesn't have an file extension ".gotmpl", this template expression should not be evaluated
baz: "{{ readFile \"baz.txt\" }}"`)

	barYamlFile := "/example/path/to/bar.yaml.gotmpl"
	barYamlContent := []byte(`foo: FOO
bar: {{ readFile "bar.txt" }}
`)

	barTextFile := "/example/path/to/bar.txt"
	barTextContent := []byte("BAR")

	expected := map[string]interface{}{
		"foo": "FOO",
		"bar": "BAR",
		// As the file doesn't have an file extension ".gotmpl", this template expression should not be evaluated
		"baz": "{{ readFile \"baz.txt\" }}",
	}

	valuesFile := "/example/path/to/values.yaml.gotmpl"
	valuesContent := []byte(`env: {{ .Environment.Name }}`)

	expectedValues := `env: production`

	readFile := func(filename string) ([]byte, error) {
		switch filename {
		case fooYamlFile:
			return fooYamlContent, nil
		case barYamlFile:
			return barYamlContent, nil
		case barTextFile:
			return barTextContent, nil
		case valuesFile:
			return valuesContent, nil
		}
		return nil, fmt.Errorf("unexpected filename: %s", filename)
	}

	state, err := NewCreator(logger, readFile, filepath.Abs).CreateFromYaml(yamlContent, yamlFile, "production")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	actual := state.env.Values
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("unexpected environment values: expected=%v, actual=%v", expected, actual)
	}

	actualValuesData, err := state.RenderValuesFileToBytes(valuesFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actualValues := string(actualValuesData)

	if !reflect.DeepEqual(expectedValues, actualValues) {
		t.Errorf("unexpected values: expected=%v, actual=%v", expectedValues, actualValues)
	}
}

func TestReadFromYaml_StrictUnmarshalling(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease
  namespace: mynamespace
  releases: mychart
`)
	_, err := createFromYaml(yamlContent, yamlFile, DefaultEnv, logger)
	if err == nil {
		t.Error("expected an error for wrong key 'releases' which is not in struct")
	}
}

func TestReadFromYaml_DeprecatedReleaseReferences(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`charts:
- name: myrelease
  chart: mychart
`)
	state, err := createFromYaml(yamlContent, yamlFile, DefaultEnv, logger)
	if err != nil {
		t.Errorf("unxpected error: %v", err)
	}

	if state.Releases[0].Name != "myrelease" {
		t.Errorf("unexpected release name: expected=myrelease actual=%s", state.Releases[0].Name)
	}
	if state.Releases[0].Chart != "mychart" {
		t.Errorf("unexpected chart name: expected=mychart actual=%s", state.Releases[0].Chart)
	}
}

func TestReadFromYaml_ConflictingReleasesConfig(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`charts:
- name: myrelease1
  chart: mychart1
releases:
- name: myrelease2
  chart: mychart2
`)
	_, err := createFromYaml(yamlContent, yamlFile, DefaultEnv, logger)
	if err == nil {
		t.Error("expected error")
	}
}

func TestReadFromYaml_FilterReleasesOnLabels(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease1
  chart: mychart1
  labels:
    tier: frontend
    foo: bar
- name: myrelease2
  chart: mychart2
  labels:
    tier: frontend
- name: myrelease3
  chart: mychart3
  labels:
    tier: backend
`)
	cases := []struct {
		filter  LabelFilter
		results []bool
	}{
		{LabelFilter{positiveLabels: [][]string{[]string{"tier", "frontend"}}},
			[]bool{true, true, false}},
		{LabelFilter{positiveLabels: [][]string{[]string{"tier", "frontend"}, []string{"foo", "bar"}}},
			[]bool{true, false, false}},
		{LabelFilter{negativeLabels: [][]string{[]string{"tier", "frontend"}}},
			[]bool{false, false, true}},
		{LabelFilter{positiveLabels: [][]string{[]string{"tier", "frontend"}}, negativeLabels: [][]string{[]string{"foo", "bar"}}},
			[]bool{false, true, false}},
	}
	state, err := createFromYaml(yamlContent, yamlFile, DefaultEnv, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	for idx, c := range cases {
		for idx2, expected := range c.results {
			if f := c.filter.Match(state.Releases[idx2]); f != expected {
				t.Errorf("[case: %d][outcome: %d] Unexpected outcome wanted %t, got %t", idx, idx2, expected, f)
			}
		}
	}
}

func TestReadFromYaml_FilterNegatives(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease1
  chart: mychart1
  labels:
    stage: pre
    foo: bar
- name: myrelease2
  chart: mychart2
  labels:
    stage: post
- name: myrelease3
  chart: mychart3
`)
	cases := []struct {
		filter  LabelFilter
		results []bool
	}{
		{LabelFilter{positiveLabels: [][]string{[]string{"stage", "pre"}}},
			[]bool{true, false, false}},
		{LabelFilter{positiveLabels: [][]string{[]string{"stage", "post"}}},
			[]bool{false, true, false}},
		{LabelFilter{negativeLabels: [][]string{[]string{"stage", "pre"}, []string{"stage", "post"}}},
			[]bool{false, false, true}},
	}
	state, err := createFromYaml(yamlContent, yamlFile, DefaultEnv, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	for idx, c := range cases {
		for idx2, expected := range c.results {
			if f := c.filter.Match(state.Releases[idx2]); f != expected {
				t.Errorf("[case: %d][outcome: %d] Unexpected outcome wanted %t, got %t", idx, idx2, expected, f)
			}
		}
	}
}
