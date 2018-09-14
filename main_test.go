package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roboll/helmfile/state"
	"gopkg.in/yaml.v2"
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

func makeRenderer(readFile func(string) ([]byte, error), env string, namespace string) *twoPassRenderer {
	return &twoPassRenderer{
		reader:    readFile,
		env:       env,
		namespace: namespace,
		filename:  "",
		logger:    logger,
		abs:       filepath.Abs,
	}
}

func TestReadFromYaml_MakeEnvironmentHasNoSideEffects(t *testing.T) {

	yamlContent := []byte(`
environments:
  staging:
    values:
    - default/values.yaml
  production:

releases:
- name: {{ readFile "other/default/values.yaml" }}
  chart: mychart1
`)

	fileReaderCalls := 0
	// make a reader that returns a simulated context
	fileReader := func(filename string) ([]byte, error) {
		expectedFilename := filepath.Clean("default/values.yaml")
		if !strings.HasSuffix(filename, expectedFilename) {
			return nil, fmt.Errorf("unexpected filename: expected=%s, actual=%s", expectedFilename, filename)
		}
		fileReaderCalls++
		if fileReaderCalls == 2 {
			return []byte("SecondPass"), nil
		}
		return []byte(""), nil
	}

	r := makeRenderer(fileReader, "staging", "namespace")
	yamlBuf, err := r.renderTemplate(yamlContent)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	var state state.HelmState
	err = yaml.Unmarshal(yamlBuf.Bytes(), &state)

	if fileReaderCalls > 2 {
		t.Error("reader should be called only twice")
	}

	if state.Releases[0].Name != "SecondPass" {
		t.Errorf("release name should have ben set as SecondPass")
	}
}

func TestReadFromYaml_RenderTemplate(t *testing.T) {

	defaultValuesYaml := []byte(`
releaseName: "hello"
conditionalReleaseTag: "yes"
`)

	yamlContent := []byte(`
environments:
  staging:
    values:
    - default/values.yaml
  production:

releases:
- name: {{ .Environment.Values.releaseName }}
  chart: mychart1

{{ if (eq .Environment.Values.conditionalReleaseTag "yes") }}
- name: conditionalRelease
{{ end }}

`)

	// make a reader that returns a simulated context
	fileReader := func(filename string) ([]byte, error) {
		expectedFilename := filepath.Clean("default/values.yaml")
		if !strings.HasSuffix(filename, expectedFilename) {
			return nil, fmt.Errorf("unexpected filename: expected=%s, actual=%s", expectedFilename, filename)
		}
		return defaultValuesYaml, nil
	}

	r := makeRenderer(fileReader, "staging", "namespace")
	// test the double rendering
	yamlBuf, err := r.renderTemplate(yamlContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var state state.HelmState
	err = yaml.Unmarshal(yamlBuf.Bytes(), &state)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(state.Releases) != 2 {
		t.Fatal("there should be 2 releases")
	}

	if state.Releases[0].Name != "hello" {
		t.Errorf("release name should be hello")
	}

	if state.Releases[1].Name != "conditionalRelease" {
		t.Error("conditional release should have been present")
	}
}

func TestReadFromYaml_RenderTemplateWithValuesReferenceError(t *testing.T) {
	defaultValuesYaml := []byte("")

	yamlContent := []byte(`
environments:
  staging:
    values:
    - default/values.yaml
  production:

{{ if (eq .Environment.Values.releaseName "a") }} # line 8
releases:
- name: a
	chart: mychart1
{{ end }}
`)

	// make a reader that returns a simulated context
	fileReader := func(filename string) ([]byte, error) {
		return defaultValuesYaml, nil
	}

	r := makeRenderer(fileReader, "staging", "namespace")
	// test the double rendering
	_, err := r.renderTemplate(yamlContent)

	if !strings.Contains(err.Error(), "stringTemplate:8") {
		t.Fatalf("error should contain a stringTemplate error (reference to unknow key) %v", err)
	}
}

// This test shows that a gotmpl reference will get rendered correctly
// even if the pre-render disables the readFile and exec functions.
// This does not apply to .gotmpl files, which is a nice side-effect.
func TestReadFromYaml_RenderTemplateWithGotmpl(t *testing.T) {

	defaultValuesYamlGotmpl := []byte(`
releaseName: {{ readFile "nonIgnoredFile" }}
`)

	yamlContent := []byte(`
environments:
  staging:
    values:
    - values.yaml.gotmpl
  production:

{{ if (eq .Environment.Values.releaseName "release-a") }} # line 8
releases:
- name: a
  chart: mychart1
{{ end }}
`)

	fileReader := func(filename string) ([]byte, error) {
		if strings.HasSuffix(filename, "nonIgnoredFile") {
			return []byte("release-a"), nil
		}
		return defaultValuesYamlGotmpl, nil
	}

	r := makeRenderer(fileReader, "staging", "namespace")
	rendered, _ := r.renderTemplate(yamlContent)

	var state state.HelmState
	yaml.Unmarshal(rendered.Bytes(), &state)

	if len(state.Releases) != 1 {
		t.Fatal("there should be 1 release")
	}

	if state.Releases[0].Name != "a" {
		t.Fatal("release should have been declared")
	}
}

func TestReadFromYaml_RenderTemplateWithNamespace(t *testing.T) {
	defaultValuesYaml := []byte(``)
	yamlContent := []byte(`releases:
- name: {{ .Namespace }}-myrelease
  chart: mychart
`)

	// make a reader that returns a simulated context
	fileReader := func(filename string) ([]byte, error) {
		return defaultValuesYaml, nil
	}

	r := makeRenderer(fileReader, "staging", "namespace")
	yamlBuf, err := r.renderTemplate(yamlContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var state state.HelmState
	err = yaml.Unmarshal(yamlBuf.Bytes(), &state)

	if state.Releases[0].Name != "namespace-myrelease" {
		t.Errorf("release name should be namespace-myrelease")
	}
}
