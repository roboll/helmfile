package main

import (
	"fmt"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// See https://github.com/roboll/helmfile/issues/193
func TestVisitDesiredStatesWithReleasesFiltered(t *testing.T) {
	absPaths := map[string]string{
		".": "/path/to",
		"/path/to/helmfile.d": "/path/to/helmfile.d",
	}
	dirs := map[string]bool{
		"helmfile.d": true,
	}
	files := map[string]string{
		"helmfile.yaml": `
helmfiles:
- helmfile.d/a*.yaml
- helmfile.d/b*.yaml
`,
		"/path/to/helmfile.d/a1.yaml": `
releases:
- name: zipkin
  chart: stable/zipkin
`,
		"/path/to/helmfile.d/a2.yaml": `
releases:
- name: prometheus
  chart: stable/prometheus
`,
		"/path/to/helmfile.d/b.yaml": `
releases:
- name: grafana
  chart: stable/grafana
`,
	}
	globMatches := map[string][]string{
		"/path/to/helmfile.d/a*.yaml": []string{"/path/to/helmfile.d/a1.yaml", "/path/to/helmfile.d/a2.yaml"},
		"/path/to/helmfile.d/b*.yaml": []string{"/path/to/helmfile.d/b.yaml"},
	}
	fileExistsAt := func(path string) bool {
		_, ok := files[path]
		return ok
	}
	directoryExistsAt := func(path string) bool {
		_, ok := dirs[path]
		return ok
	}
	readFile := func(filename string) ([]byte, error) {
		str, ok := files[filename]
		if !ok {
			return []byte(nil), fmt.Errorf("no file found: %s", filename)
		}
		return []byte(str), nil
	}
	glob := func(pattern string) ([]string, error) {
		matches, ok := globMatches[pattern]
		if !ok {
			return []string(nil), fmt.Errorf("no file matched: %s", pattern)
		}
		return matches, nil
	}
	abs := func(path string) (string, error) {
		a, ok := absPaths[path]
		if !ok {
			return "", fmt.Errorf("abs: unexpected path: %s", path)
		}
		return a, nil
	}
	noop := func(st *state.HelmState, helm helmexec.Interface) []error {
		return []error{}
	}

	testcases := []struct {
		name      string
		expectErr bool
	}{
		{name: "prometheus", expectErr: false},
		{name: "zipkin", expectErr: false},
		{name: "grafana", expectErr: false},
		{name: "elasticsearch", expectErr: true},
	}

	for _, testcase := range testcases {
		app := &app{
			readFile:          readFile,
			glob:              glob,
			abs:               abs,
			fileExistsAt:      fileExistsAt,
			directoryExistsAt: directoryExistsAt,
			kubeContext:       "default",
			logger:            helmexec.NewLogger(os.Stderr, "debug"),
			selectors:         []string{fmt.Sprintf("name=%s", testcase.name)},
			namespace:         "",
			env:               "default",
		}
		err := app.VisitDesiredStatesWithReleasesFiltered(
			"helmfile.yaml", noop,
		)
		if testcase.expectErr && err == nil {
			t.Errorf("error expected but not happened for name=%s", testcase.name)
		} else if !testcase.expectErr && err != nil {
			t.Errorf("unexpected error for name=%s: %v", testcase.name, err)
		}
	}
}

// See https://github.com/roboll/helmfile/issues/320
func TestVisitDesiredStatesWithReleasesFiltered_UndefinedEnv(t *testing.T) {
	absPaths := map[string]string{
		".": "/path/to",
		"/path/to/helmfile.d": "/path/to/helmfile.d",
	}
	dirs := map[string]bool{
		"helmfile.d": true,
	}
	files := map[string]string{
		"helmfile.yaml": `
environments:
  prod:

helmfiles:
- helmfile.d/a*.yaml
`,
		"/path/to/helmfile.d/a1.yaml": `
environments:
  prod:

releases:
- name: zipkin
  chart: stable/zipkin
`,
	}
	globMatches := map[string][]string{
		"/path/to/helmfile.d/a*.yaml": []string{"/path/to/helmfile.d/a1.yaml"},
	}
	fileExistsAt := func(path string) bool {
		_, ok := files[path]
		return ok
	}
	directoryExistsAt := func(path string) bool {
		_, ok := dirs[path]
		return ok
	}
	readFile := func(filename string) ([]byte, error) {
		str, ok := files[filename]
		if !ok {
			return []byte(nil), fmt.Errorf("no file found: %s", filename)
		}
		return []byte(str), nil
	}
	glob := func(pattern string) ([]string, error) {
		matches, ok := globMatches[pattern]
		if !ok {
			return []string(nil), fmt.Errorf("no file matched: %s", pattern)
		}
		return matches, nil
	}
	abs := func(path string) (string, error) {
		a, ok := absPaths[path]
		if !ok {
			return "", fmt.Errorf("abs: unexpected path: %s", path)
		}
		return a, nil
	}
	noop := func(st *state.HelmState, helm helmexec.Interface) []error {
		return []error{}
	}

	testcases := []struct {
		name      string
		expectErr bool
	}{
		{name: "undefined_env", expectErr: true},
		{name: "default", expectErr: false},
		{name: "prod", expectErr: false},
	}

	for _, testcase := range testcases {
		app := &app{
			readFile:          readFile,
			glob:              glob,
			abs:               abs,
			fileExistsAt:      fileExistsAt,
			directoryExistsAt: directoryExistsAt,
			kubeContext:       "default",
			logger:            helmexec.NewLogger(os.Stderr, "debug"),
			namespace:         "",
			selectors:         []string{},
			env:               testcase.name,
		}
		err := app.VisitDesiredStatesWithReleasesFiltered(
			"helmfile.yaml", noop,
		)
		if testcase.expectErr && err == nil {
			t.Errorf("error expected but not happened for environment=%s", testcase.name)
		} else if !testcase.expectErr && err != nil {
			t.Errorf("unexpected error for environment=%s: %v", testcase.name, err)
		}
	}
}

// See https://github.com/roboll/helmfile/issues/322
func TestVisitDesiredStatesWithReleasesFiltered_Selectors(t *testing.T) {
	absPaths := map[string]string{
		".": "/path/to",
		"/path/to/helmfile.d": "/path/to/helmfile.d",
	}
	dirs := map[string]bool{
		"helmfile.d": true,
	}
	files := map[string]string{
		"helmfile.yaml": `
helmfiles:
- helmfile.d/a*.yaml
- helmfile.d/b*.yaml
`,
		"/path/to/helmfile.d/a1.yaml": `
releases:
- name: zipkin
  chart: stable/zipkin
`,
		"/path/to/helmfile.d/a2.yaml": `
releases:
- name: prometheus
  chart: stable/prometheus
`,
		"/path/to/helmfile.d/b.yaml": `
releases:
- name: grafana
  chart: stable/grafana
- name: foo
  chart: charts/foo
  labels:
    duplicated: yes
- name: foo
  chart: charts/foo
  labels:
    duplicated: yes
`,
	}
	globMatches := map[string][]string{
		"/path/to/helmfile.d/a*.yaml": []string{"/path/to/helmfile.d/a1.yaml", "/path/to/helmfile.d/a2.yaml"},
		"/path/to/helmfile.d/b*.yaml": []string{"/path/to/helmfile.d/b.yaml"},
	}
	fileExistsAt := func(path string) bool {
		_, ok := files[path]
		return ok
	}
	directoryExistsAt := func(path string) bool {
		_, ok := dirs[path]
		return ok
	}
	readFile := func(filename string) ([]byte, error) {
		str, ok := files[filename]
		if !ok {
			return []byte(nil), fmt.Errorf("no file found: %s", filename)
		}
		return []byte(str), nil
	}
	glob := func(pattern string) ([]string, error) {
		matches, ok := globMatches[pattern]
		if !ok {
			return []string(nil), fmt.Errorf("no file matched: %s", pattern)
		}
		return matches, nil
	}
	abs := func(path string) (string, error) {
		a, ok := absPaths[path]
		if !ok {
			return "", fmt.Errorf("abs: unexpected path: %s", path)
		}
		return a, nil
	}

	testcases := []struct {
		label         string
		expectedCount int
		expectErr     bool
		errMsg        string
	}{
		{label: "name=prometheus", expectedCount: 1, expectErr: false},
		{label: "name=", expectedCount: 0, expectErr: true, errMsg: "failed processing /path/to/helmfile.d/a1.yaml: Malformed label: name=. Expected label in form k=v or k!=v"},
		{label: "name!=", expectedCount: 0, expectErr: true, errMsg: "failed processing /path/to/helmfile.d/a1.yaml: Malformed label: name!=. Expected label in form k=v or k!=v"},
		{label: "name", expectedCount: 0, expectErr: true, errMsg: "failed processing /path/to/helmfile.d/a1.yaml: Malformed label: name. Expected label in form k=v or k!=v"},
		// See https://github.com/roboll/helmfile/issues/193
		{label: "duplicated=yes", expectedCount: 0, expectErr: true, errMsg: "failed processing /path/to/helmfile.d/b.yaml: duplicate release \"foo\" found: there were 2 releases named \"foo\" matching specified selector"},
	}

	for _, testcase := range testcases {
		actual := []string{}

		collectReleases := func(st *state.HelmState, helm helmexec.Interface) []error {
			for _, r := range st.Releases {
				actual = append(actual, r.Name)
			}
			return []error{}
		}

		app := &app{
			readFile:          readFile,
			glob:              glob,
			abs:               abs,
			fileExistsAt:      fileExistsAt,
			directoryExistsAt: directoryExistsAt,
			kubeContext:       "default",
			logger:            helmexec.NewLogger(os.Stderr, "debug"),
			namespace:         "",
			selectors:         []string{testcase.label},
			env:               "default",
		}

		err := app.VisitDesiredStatesWithReleasesFiltered(
			"helmfile.yaml", collectReleases,
		)
		if testcase.expectErr {
			if err == nil {
				t.Errorf("error expected but not happened for selector %s", testcase.label)
			} else if err.Error() != testcase.errMsg {
				t.Errorf("unexpected error message: expected=\"%s\", actual=\"%s\"", testcase.errMsg, err.Error())
			}
		} else if !testcase.expectErr && err != nil {
			t.Errorf("unexpected error for selector %s: %v", testcase.label, err)
		}
		if len(actual) != testcase.expectedCount {
			t.Errorf("unexpected release count for selector %s: expected=%d, actual=%d", testcase.label, testcase.expectedCount, len(actual))
		}
	}
}

// See https://github.com/roboll/helmfile/issues/312
func TestVisitDesiredStatesWithReleasesFiltered_ReverseOrder(t *testing.T) {
	absPaths := map[string]string{
		".": "/path/to",
		"/path/to/helmfile.d": "/path/to/helmfile.d",
	}
	dirs := map[string]bool{
		"helmfile.d": true,
	}
	files := map[string]string{
		"helmfile.yaml": `
helmfiles:
- helmfile.d/a*.yaml
- helmfile.d/b*.yaml
`,
		"/path/to/helmfile.d/a1.yaml": `
releases:
- name: zipkin
  chart: stable/zipkin
`,
		"/path/to/helmfile.d/a2.yaml": `
releases:
- name: prometheus
  chart: stable/prometheus
- name: elasticsearch
  chart: stable/elasticsearch
`,
		"/path/to/helmfile.d/b.yaml": `
releases:
- name: grafana
  chart: stable/grafana
`,
	}
	globMatches := map[string][]string{
		"/path/to/helmfile.d/a*.yaml": []string{"/path/to/helmfile.d/a1.yaml", "/path/to/helmfile.d/a2.yaml"},
		"/path/to/helmfile.d/b*.yaml": []string{"/path/to/helmfile.d/b.yaml"},
	}
	fileExistsAt := func(path string) bool {
		_, ok := files[path]
		return ok
	}
	directoryExistsAt := func(path string) bool {
		_, ok := dirs[path]
		return ok
	}
	readFile := func(filename string) ([]byte, error) {
		str, ok := files[filename]
		if !ok {
			return []byte(nil), fmt.Errorf("no file found: %s", filename)
		}
		return []byte(str), nil
	}
	glob := func(pattern string) ([]string, error) {
		matches, ok := globMatches[pattern]
		if !ok {
			return []string(nil), fmt.Errorf("no file matched: %s", pattern)
		}
		return matches, nil
	}
	abs := func(path string) (string, error) {
		a, ok := absPaths[path]
		if !ok {
			return "", fmt.Errorf("abs: unexpected path: %s", path)
		}
		return a, nil
	}

	expected := []string{"grafana", "elasticsearch", "prometheus", "zipkin"}

	testcases := []struct {
		reverse  bool
		expected []string
	}{
		{reverse: false, expected: []string{"zipkin", "prometheus", "elasticsearch", "grafana"}},
		{reverse: true, expected: []string{"grafana", "elasticsearch", "prometheus", "zipkin"}},
	}
	for _, testcase := range testcases {
		actual := []string{}

		collectReleases := func(st *state.HelmState, helm helmexec.Interface) []error {
			for _, r := range st.Releases {
				actual = append(actual, r.Name)
			}
			return []error{}
		}

		app := &app{
			readFile:          readFile,
			glob:              glob,
			abs:               abs,
			fileExistsAt:      fileExistsAt,
			directoryExistsAt: directoryExistsAt,
			kubeContext:       "default",
			logger:            helmexec.NewLogger(os.Stderr, "debug"),
			reverse:           testcase.reverse,
			namespace:         "",
			selectors:         []string{},
			env:               "default",
		}
		err := app.VisitDesiredStatesWithReleasesFiltered(
			"helmfile.yaml", collectReleases,
		)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(testcase.expected, actual) {
			t.Errorf("releases did not match: expected=%v actual=%v", expected, actual)
		}
	}
}

func TestLoadDesiredStateFromYaml_DuplicateReleaseName(t *testing.T) {
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
	_, err := app.loadDesiredStateFromYaml(yamlContent, yamlFile, "default", "default")
	if err != nil {
		t.Error("unexpected error")
	}
}
