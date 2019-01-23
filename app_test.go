package main

import (
	"fmt"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type testFs struct {
	wd    string
	dirs  map[string]bool
	files map[string]string
}

func appWithFs(app *app, files map[string]string) *app {
	fs := newTestFs(files)
	return injectFs(app, fs)
}

func injectFs(app *app, fs *testFs) *app {
	app.readFile = fs.readFile
	app.glob = fs.glob
	app.abs = fs.abs
	app.getwd = fs.getwd
	app.chdir = fs.chdir
	app.fileExistsAt = fs.fileExistsAt
	app.directoryExistsAt = fs.directoryExistsAt
	return app
}

func newTestFs(files map[string]string) *testFs {
	dirs := map[string]bool{}
	for abs, _ := range files {
		d := filepath.Dir(abs)
		dirs[d] = true
	}
	return &testFs{
		wd:    "/path/to",
		dirs:  dirs,
		files: files,
	}
}

func (f *testFs) fileExistsAt(path string) bool {
	var ok bool
	if strings.Contains(path, "/") {
		_, ok = f.files[path]
	} else {
		_, ok = f.files[filepath.Join(f.wd, path)]
	}
	return ok
}

func (f *testFs) directoryExistsAt(path string) bool {
	var ok bool
	if strings.Contains(path, "/") {
		_, ok = f.dirs[path]
	} else {
		_, ok = f.dirs[filepath.Join(f.wd, path)]
	}
	return ok
}

func (f *testFs) readFile(filename string) ([]byte, error) {
	var str string
	var ok bool
	if strings.Contains(filename, "/") {
		str, ok = f.files[filename]
	} else {
		str, ok = f.files[filepath.Join(f.wd, filename)]
	}
	if !ok {
		return []byte(nil), fmt.Errorf("no file found: %s", filename)
	}
	return []byte(str), nil
}

func (f *testFs) glob(pattern string) ([]string, error) {
	matches := []string{}
	for name, _ := range f.files {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return []string(nil), fmt.Errorf("no file matched: %s", pattern)
	}
	return matches, nil
}

func (f *testFs) abs(path string) (string, error) {
	var p string
	if path[0] == '/' {
		p = path
	} else {
		p = filepath.Join(f.wd, path)
	}
	return filepath.Clean(p), nil
}

func (f *testFs) getwd() (string, error) {
	return f.wd, nil
}

func (f *testFs) chdir(dir string) error {
	if dir == "/path/to" || dir == "/path/to/helmfile.d" {
		f.wd = dir
		return nil
	}
	return fmt.Errorf("unexpected chdir \"%s\"", dir)
}

// See https://github.com/roboll/helmfile/issues/193
func TestVisitDesiredStatesWithReleasesFiltered(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
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
		app := appWithFs(&app{
			kubeContext: "default",
			logger:      helmexec.NewLogger(os.Stderr, "debug"),
			selectors:   []string{fmt.Sprintf("name=%s", testcase.name)},
			namespace:   "",
			env:         "default",
		}, files)
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
	files := map[string]string{
		"/path/to/helmfile.yaml": `
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
		app := appWithFs(&app{
			kubeContext: "default",
			logger:      helmexec.NewLogger(os.Stderr, "debug"),
			namespace:   "",
			selectors:   []string{},
			env:         testcase.name,
		}, files)
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
	files := map[string]string{
		"/path/to/helmfile.yaml": `
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

		app := appWithFs(&app{
			kubeContext: "default",
			logger:      helmexec.NewLogger(os.Stderr, "debug"),
			namespace:   "",
			selectors:   []string{testcase.label},
			env:         "default",
		}, files)

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
	files := map[string]string{
		"/path/to/helmfile.yaml": `
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
		app := appWithFs(&app{
			kubeContext: "default",
			logger:      helmexec.NewLogger(os.Stderr, "debug"),
			reverse:     testcase.reverse,
			namespace:   "",
			selectors:   []string{},
			env:         "default",
		}, files)
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
