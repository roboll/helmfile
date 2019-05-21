package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"gotest.tools/env"
)

func appWithFs(app *App, files map[string]string) *App {
	fs := state.NewTestFs(files)
	return injectFs(app, fs)
}

func injectFs(app *App, fs *state.TestFs) *App {
	app.readFile = fs.ReadFile
	app.glob = fs.Glob
	app.abs = fs.Abs
	app.getwd = fs.Getwd
	app.chdir = fs.Chdir
	app.fileExistsAt = fs.FileExistsAt
	app.directoryExistsAt = fs.DirectoryExistsAt
	return app
}

func TestVisitDesiredStatesWithReleasesFiltered_ReleaseOrder(t *testing.T) {
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
	fs := state.NewTestFs(files)
	fs.GlobFixtures["/path/to/helmfile.d/a*.yaml"] = []string{"/path/to/helmfile.d/a2.yaml", "/path/to/helmfile.d/a1.yaml"}
	app := &App{
		KubeContext: "default",
		Logger:      helmexec.NewLogger(os.Stderr, "debug"),
		Namespace:   "",
		Env:         "default",
	}
	app = injectFs(app, fs)
	actualOrder := []string{}
	noop := func(st *state.HelmState, helm helmexec.Interface) []error {
		actualOrder = append(actualOrder, st.FilePath)
		return []error{}
	}

	err := app.VisitDesiredStatesWithReleasesFiltered(
		"helmfile.yaml", noop,
	)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expectedOrder := []string{"a1.yaml", "a2.yaml", "b.yaml", "helmfile.yaml"}
	if !reflect.DeepEqual(actualOrder, expectedOrder) {
		t.Errorf("unexpected order of processed state files: expected=%v, actual=%v", expectedOrder, actualOrder)
	}
}

func TestVisitDesiredStatesWithReleasesFiltered_EnvValuesFileOrder(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
environments:
  default:
    values:
    - env.*.yaml
releases:
- name: zipkin
  chart: stable/zipkin
`,
		"/path/to/env.1.yaml": `FOO: 1
BAR: 2
`,
		"/path/to/env.2.yaml": `BAR: 3
BAZ: 4
`,
	}
	fs := state.NewTestFs(files)
	fs.GlobFixtures["/path/to/env.*.yaml"] = []string{"/path/to/env.2.yaml", "/path/to/env.1.yaml"}
	app := &App{
		KubeContext: "default",
		Logger:      helmexec.NewLogger(os.Stderr, "debug"),
		Namespace:   "",
		Env:         "default",
	}
	app = injectFs(app, fs)
	noop := func(st *state.HelmState, helm helmexec.Interface) []error {
		return []error{}
	}

	err := app.VisitDesiredStatesWithReleasesFiltered(
		"helmfile.yaml", noop,
	)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expectedOrder := []string{"helmfile.yaml", "/path/to/env.1.yaml", "/path/to/env.2.yaml", "/path/to/env.1.yaml", "/path/to/env.2.yaml"}
	actualOrder := fs.SuccessfulReads()
	if !reflect.DeepEqual(actualOrder, expectedOrder) {
		t.Errorf("unexpected order of processed state files: expected=%v, actual=%v", expectedOrder, actualOrder)
	}
}

func TestVisitDesiredStatesWithReleasesFiltered_MissingEnvValuesFile(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
environments:
  default:
    values:
    - env.*.yaml
releases:
- name: zipkin
  chart: stable/zipkin
`,
	}
	fs := state.NewTestFs(files)
	app := &App{
		KubeContext: "default",
		Logger:      helmexec.NewLogger(os.Stderr, "debug"),
		Namespace:   "",
		Env:         "default",
	}
	app = injectFs(app, fs)
	noop := func(st *state.HelmState, helm helmexec.Interface) []error {
		return []error{}
	}

	err := app.VisitDesiredStatesWithReleasesFiltered(
		"helmfile.yaml", noop,
	)
	if err == nil {
		t.Fatal("expected error did not occur")
	}

	expected := "in ./helmfile.yaml: failed to read helmfile.yaml: no file matching env.*.yaml found"
	if err.Error() != expected {
		t.Errorf("unexpected error: expected=%s, got=%v", expected, err)
	}
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
		fs := state.NewTestFs(files)
		fs.GlobFixtures["/path/to/helmfile.d/a*.yaml"] = []string{"/path/to/helmfile.d/a2.yaml", "/path/to/helmfile.d/a1.yaml"}
		app := &App{
			KubeContext: "default",
			Logger:      helmexec.NewLogger(os.Stderr, "debug"),
			Selectors:   []string{fmt.Sprintf("name=%s", testcase.name)},
			Namespace:   "",
			Env:         "default",
		}
		app = injectFs(app, fs)
		noop := func(st *state.HelmState, helm helmexec.Interface) []error {
			return []error{}
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
		app := appWithFs(&App{
			KubeContext: "default",
			Logger:      helmexec.NewLogger(os.Stderr, "debug"),
			Namespace:   "",
			Selectors:   []string{},
			Env:         testcase.name,
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
helmDefaults:
  tillerNamespace: zoo
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
- name: bar
  chart: charts/foo
  tillerNamespace:  bar1
  labels:
    duplicatedOK: yes
- name: bar
  chart: charts/foo
  tillerNamespace: bar2
  labels:
    duplicatedOK: yes
`,
	}

	testcases := []struct {
		label         string
		expectedCount int
		expectErr     bool
		errMsg        string
	}{
		{label: "name=prometheus", expectedCount: 1, expectErr: false},
		{label: "name=", expectedCount: 0, expectErr: true, errMsg: "in ./helmfile.yaml: in .helmfiles[0]: in /path/to/helmfile.d/a1.yaml: Malformed label: name=. Expected label in form k=v or k!=v"},
		{label: "name!=", expectedCount: 0, expectErr: true, errMsg: "in ./helmfile.yaml: in .helmfiles[0]: in /path/to/helmfile.d/a1.yaml: Malformed label: name!=. Expected label in form k=v or k!=v"},
		{label: "name", expectedCount: 0, expectErr: true, errMsg: "in ./helmfile.yaml: in .helmfiles[0]: in /path/to/helmfile.d/a1.yaml: Malformed label: name. Expected label in form k=v or k!=v"},
		// See https://github.com/roboll/helmfile/issues/193
		{label: "duplicated=yes", expectedCount: 0, expectErr: true, errMsg: "in ./helmfile.yaml: in .helmfiles[2]: in /path/to/helmfile.d/b.yaml: duplicate release \"foo\" found in \"zoo\": there were 2 releases named \"foo\" matching specified selector"},
		{label: "duplicatedOK=yes", expectedCount: 2, expectErr: false},
	}

	for _, testcase := range testcases {
		actual := []string{}

		collectReleases := func(st *state.HelmState, helm helmexec.Interface) []error {
			for _, r := range st.Releases {
				actual = append(actual, r.Name)
			}
			return []error{}
		}

		app := appWithFs(&App{
			KubeContext: "default",
			Logger:      helmexec.NewLogger(os.Stderr, "debug"),
			Namespace:   "",
			Selectors:   []string{testcase.label},
			Env:         "default",
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

func TestVisitDesiredStatesWithReleasesFiltered_EmbeddedSelectors(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
helmfiles:
- path: helmfile.d/a*.yaml
  selectors:
  - name=prometheus      
  - name=zipkin      
- helmfile.d/b*.yaml
- path: helmfile.d/c*.yaml
  selectors: []
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
		"/path/to/helmfile.d/a3.yaml": `
releases:
- name: mongodb
  chart: stable/mongodb
`,
		"/path/to/helmfile.d/b.yaml": `
releases:
- name: grafana
  chart: stable/grafana
- name: bar
  chart: charts/foo
  tillerNamespace:  bar1
  labels:
    duplicatedOK: yes
- name: bar
  chart: charts/foo
  tillerNamespace: bar2
  labels:
    duplicatedOK: yes
`,
		"/path/to/helmfile.d/c.yaml": `
releases:
- name: grafana
  chart: stable/grafana
- name: postgresql
  chart: charts/postgresql
  labels:
    whatever: yes
`,
	}
	//Check with legacy behavior, that is when no explicit selector then sub-helmfiles inherits from command line selector
	legacyTestcases := []struct {
		label            string
		expectedReleases []string
		expectErr        bool
		errMsg           string
	}{
		{label: "duplicatedOK=yes", expectedReleases: []string{"zipkin", "prometheus", "bar", "bar", "grafana", "postgresql"}, expectErr: false},
		{label: "name=zipkin", expectedReleases: []string{"zipkin", "prometheus", "grafana", "postgresql"}, expectErr: false},
		{label: "name=grafana", expectedReleases: []string{"zipkin", "prometheus", "grafana", "grafana", "postgresql"}, expectErr: false},
		{label: "name=doesnotexists", expectedReleases: []string{"zipkin", "prometheus", "grafana", "postgresql"}, expectErr: false},
	}
	runFilterSubHelmFilesTests(legacyTestcases, files, t, "1st EmbeddedSelectors")

	//Check with experimental behavior, that is when no explicit selector then sub-helmfiles do no inherit from any selector
	desiredTestcases := []struct {
		label            string
		expectedReleases []string
		expectErr        bool
		errMsg           string
	}{
		{label: "duplicatedOK=yes", expectedReleases: []string{"zipkin", "prometheus", "grafana", "bar", "bar", "grafana", "postgresql"}, expectErr: false},
		{label: "name=doesnotexists", expectedReleases: []string{"zipkin", "prometheus", "grafana", "bar", "bar", "grafana", "postgresql"}, expectErr: false},
	}

	defer env.Patch(t, ExperimentalEnvVar, ExperimentalSelectorExplicit)()

	runFilterSubHelmFilesTests(desiredTestcases, files, t, "2nd EmbeddedSelectors")

}

func TestVisitDesiredStatesWithReleasesFiltered_InheritedSelectors_3leveldeep(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
helmfiles:
- helmfile.d/a*.yaml
releases:
- name: mongodb
  chart: stable/mongodb
`,
		"/path/to/helmfile.d/a.yaml": `
helmfiles:
- b*.yaml
releases:
- name: zipkin
  chart: stable/zipkin
`,
		"/path/to/helmfile.d/b.yaml": `
releases:
- name: grafana
  chart: stable/grafana
`,
	}
	//Check with legacy behavior, that is when no explicit selector then sub-helmfiles inherits from command line selector
	legacyTestcases := []struct {
		label            string
		expectedReleases []string
		expectErr        bool
		errMsg           string
	}{
		{label: "name!=grafana", expectedReleases: []string{"zipkin", "mongodb"}, expectErr: false},
	}
	runFilterSubHelmFilesTests(legacyTestcases, files, t, "1st 3leveldeep")

	//Check with experimental behavior, that is when no explicit selector then sub-helmfiles do no inherit from any selector
	desiredTestcases := []struct {
		label            string
		expectedReleases []string
		expectErr        bool
		errMsg           string
	}{
		{label: "name!=grafana", expectedReleases: []string{"grafana", "zipkin", "mongodb"}, expectErr: false},
	}

	defer env.Patch(t, ExperimentalEnvVar, ExperimentalSelectorExplicit)()

	runFilterSubHelmFilesTests(desiredTestcases, files, t, "2nd 3leveldeep")

}

func TestVisitDesiredStatesWithReleasesFiltered_InheritedSelectors_inherits(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
helmfiles:
- helmfile.d/a*.yaml
- path: helmfile.d/a*.yaml
  selectors:
  - select=foo
releases:
- name: mongodb
  chart: stable/mongodb
`,
		"/path/to/helmfile.d/a.yaml": `
helmfiles:
- path: b*.yaml
  selectorsInherited: true
releases:
- name: zipkin
  chart: stable/zipkin
  labels:
    select: foo
`,
		"/path/to/helmfile.d/b.yaml": `
releases:
- name: grafana
  chart: stable/grafana
- name: prometheus
  chart: stable/prometheus
  labels:
    select: foo
`,
	}
	//Check with legacy behavior, that is when no explicit selector then sub-helmfiles inherits from command line selector
	legacyTestcases := []struct {
		label            string
		expectedReleases []string
		expectErr        bool
		errMsg           string
	}{
		{label: "name=grafana", expectedReleases: []string{"grafana", "prometheus", "zipkin"}, expectErr: false},
		{label: "select!=foo", expectedReleases: []string{"grafana", "prometheus", "zipkin", "mongodb"}, expectErr: false},
	}
	runFilterSubHelmFilesTests(legacyTestcases, files, t, "1st inherits")

	//Check with experimental behavior, that is when no explicit selector then sub-helmfiles do no inherit from any selector
	desiredTestcases := []struct {
		label            string
		expectedReleases []string
		expectErr        bool
		errMsg           string
	}{
		{label: "name=grafana", expectedReleases: []string{"grafana", "prometheus", "zipkin", "prometheus", "zipkin"}, expectErr: false},
		{label: "select!=foo", expectedReleases: []string{"grafana", "prometheus", "zipkin", "prometheus", "zipkin", "mongodb"}, expectErr: false},
	}

	defer env.Patch(t, ExperimentalEnvVar, ExperimentalSelectorExplicit)()

	runFilterSubHelmFilesTests(desiredTestcases, files, t, "2nd inherits")

}

func runFilterSubHelmFilesTests(testcases []struct {
	label            string
	expectedReleases []string
	expectErr        bool
	errMsg           string
}, files map[string]string, t *testing.T, testName string) {
	for _, testcase := range testcases {
		actual := []string{}

		collectReleases := func(st *state.HelmState, helm helmexec.Interface) []error {
			for _, r := range st.Releases {
				actual = append(actual, r.Name)
			}
			return []error{}
		}

		app := appWithFs(&App{
			KubeContext: "default",
			Logger:      helmexec.NewLogger(os.Stderr, "debug"),
			Namespace:   "",
			Selectors:   []string{testcase.label},
			Env:         "default",
		}, files)

		err := app.VisitDesiredStatesWithReleasesFiltered(
			"helmfile.yaml", collectReleases,
		)
		if testcase.expectErr {
			if err == nil {
				t.Errorf("[%s]error expected but not happened for selector %s", testName, testcase.label)
			} else if err.Error() != testcase.errMsg {
				t.Errorf("[%s]unexpected error message: expected=\"%s\", actual=\"%s\"", testName, testcase.errMsg, err.Error())
			}
		} else if !testcase.expectErr && err != nil {
			t.Errorf("[%s]unexpected error for selector %s: %v", testName, testcase.label, err)
		}
		if !reflect.DeepEqual(actual, testcase.expectedReleases) {
			t.Errorf("[%s]unexpected releases for selector %s: expected=%v, actual=%v", testName, testcase.label, testcase.expectedReleases, actual)
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
		app := appWithFs(&App{
			KubeContext: "default",
			Logger:      helmexec.NewLogger(os.Stderr, "debug"),
			Reverse:     testcase.reverse,
			Namespace:   "",
			Selectors:   []string{},
			Env:         "default",
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
	readFile := func(filename string) ([]byte, error) {
		if filename != yamlFile {
			return nil, fmt.Errorf("unexpected filename: %s", filename)
		}
		return yamlContent, nil
	}
	app := &App{
		readFile:    readFile,
		glob:        filepath.Glob,
		abs:         filepath.Abs,
		KubeContext: "default",
		Env:         "default",
		Logger:      helmexec.NewLogger(os.Stderr, "debug"),
	}
	_, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadDesiredStateFromYaml_Bases(t *testing.T) {
	yamlFile := "/path/to/yaml/file"
	yamlContent := `bases:
- ../base.yaml
- ../base.gotmpl

{{ readFile "templates.yaml" }}

releases:
- name: myrelease1
  chart: mychart1
  labels:
    stage: pre
    foo: bar
- name: myrelease1
  chart: mychart2
  labels:
    stage: post
  <<: *default
`
	testFs := state.NewTestFs(map[string]string{
		yamlFile: yamlContent,
		"/path/to/base.yaml": `environments:
  default:
    values:
    - environments/default/1.yaml
`,
		"/path/to/yaml/environments/default/1.yaml": `foo: FOO`,
		"/path/to/base.gotmpl": `environments:
  default:
    values:
    - environments/default/2.yaml

helmDefaults:
  tillerNamespace: {{ .Environment.Values.tillerNs }}
`,
		"/path/to/yaml/environments/default/2.yaml": `tillerNs: TILLER_NS`,
		"/path/to/yaml/templates.yaml": `templates:
  default: &default
    missingFileHandler: Warn
    values: ["` + "{{`" + `{{.Release.Name}}` + "`}}" + `/values.yaml"]
`,
	})
	app := &App{
		readFile:    testFs.ReadFile,
		glob:        testFs.Glob,
		abs:         testFs.Abs,
		KubeContext: "default",
		Env:         "default",
		Logger:      helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if st.HelmDefaults.TillerNamespace != "TILLER_NS" {
		t.Errorf("unexpected helmDefaults.tillerNamespace: expected=TILLER_NS, got=%s", st.HelmDefaults.TillerNamespace)
	}

	if *st.Releases[1].MissingFileHandler != "Warn" {
		t.Errorf("unexpected releases[0].missingFileHandler: expected=Warn, got=%s", *st.Releases[1].MissingFileHandler)
	}

	if st.Releases[1].Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[0].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", st.Releases[1].Values[0])
	}
}

func TestLoadDesiredStateFromYaml_MultiPartTemplate(t *testing.T) {
	yamlFile := "/path/to/yaml/file"
	yamlContent := `bases:
- ../base.yaml
---
bases:
- ../base.gotmpl
---
helmDefaults:
  kubeContext: {{ .Environment.Values.foo }}
---
releases:
- name: myrelease0
  chart: mychart0
---

{{ readFile "templates.yaml" }}

releases:
- name: myrelease1
  chart: mychart1
  labels:
    stage: pre
    foo: bar
- name: myrelease1
  chart: mychart2
  labels:
    stage: post
  <<: *default
`
	testFs := state.NewTestFs(map[string]string{
		yamlFile: yamlContent,
		"/path/to/base.yaml": `environments:
  default:
    values:
    - environments/default/1.yaml
`,
		"/path/to/yaml/environments/default/1.yaml": `foo: FOO`,
		"/path/to/base.gotmpl": `environments:
  default:
    values:
    - environments/default/2.yaml

helmDefaults:
  tillerNamespace: {{ .Environment.Values.tillerNs }}
`,
		"/path/to/yaml/environments/default/2.yaml": `tillerNs: TILLER_NS`,
		"/path/to/yaml/templates.yaml": `templates:
  default: &default
    missingFileHandler: Warn
    values: ["` + "{{`" + `{{.Release.Name}}` + "`}}" + `/values.yaml"]
`,
	})
	app := &App{
		readFile: testFs.ReadFile,
		glob:     testFs.Glob,
		abs:      testFs.Abs,
		Env:      "default",
		Logger:   helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if st.HelmDefaults.TillerNamespace != "TILLER_NS" {
		t.Errorf("unexpected helmDefaults.tillerNamespace: expected=TILLER_NS, got=%s", st.HelmDefaults.TillerNamespace)
	}

	if st.Releases[0].Name != "myrelease0" {
		t.Errorf("unexpected releases[0].name: expected=myrelease0, got=%s", st.Releases[0].Name)
	}
	if st.Releases[1].Name != "myrelease1" {
		t.Errorf("unexpected releases[1].name: expected=myrelease1, got=%s", st.Releases[1].Name)
	}
	if st.Releases[2].Name != "myrelease1" {
		t.Errorf("unexpected releases[2].name: expected=myrelease1, got=%s", st.Releases[2].Name)
	}
	if st.Releases[2].Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", st.Releases[1].Values[0])
	}
	if *st.Releases[2].MissingFileHandler != "Warn" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected=Warn, got=%s", *st.Releases[1].MissingFileHandler)
	}

	if st.Releases[2].Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", st.Releases[1].Values[0])
	}

	if st.HelmDefaults.KubeContext != "FOO" {
		t.Errorf("unexpected helmDefaults.kubeContext: expected=FOO, got=%s", st.HelmDefaults.KubeContext)
	}
}

func TestLoadDesiredStateFromYaml_MultiPartTemplate_WithNonDefaultEnv(t *testing.T) {
	yamlFile := "/path/to/yaml/file"
	yamlContent := `bases:
- ../base.yaml
---
bases:
- ../base.gotmpl
---
helmDefaults:
  kubeContext: {{ .Environment.Values.foo }}
---
releases:
- name: myrelease0
  chart: mychart0
---

{{ readFile "templates.yaml" }}

releases:
- name: myrelease1
  chart: mychart1
  labels:
    stage: pre
    foo: bar
- name: myrelease1
  chart: mychart2
  labels:
    stage: post
  <<: *default
`
	testFs := state.NewTestFs(map[string]string{
		yamlFile: yamlContent,
		"/path/to/base.yaml": `environments:
  test:
    values:
    - environments/default/1.yaml
`,
		"/path/to/yaml/environments/default/1.yaml": `foo: FOO`,
		"/path/to/base.gotmpl": `environments:
  test:
    values:
    - environments/default/2.yaml

helmDefaults:
  tillerNamespace: {{ .Environment.Values.tillerNs }}
`,
		"/path/to/yaml/environments/default/2.yaml": `tillerNs: TILLER_NS`,
		"/path/to/yaml/templates.yaml": `templates:
  default: &default
    missingFileHandler: Warn
    values: ["` + "{{`" + `{{.Release.Name}}` + "`}}" + `/values.yaml"]
`,
	})
	app := &App{
		readFile: testFs.ReadFile,
		glob:     testFs.Glob,
		abs:      testFs.Abs,
		Env:      "test",
		Logger:   helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.HelmDefaults.TillerNamespace != "TILLER_NS" {
		t.Errorf("unexpected helmDefaults.tillerNamespace: expected=TILLER_NS, got=%s", st.HelmDefaults.TillerNamespace)
	}

	if st.Releases[0].Name != "myrelease0" {
		t.Errorf("unexpected releases[0].name: expected=myrelease0, got=%s", st.Releases[0].Name)
	}
	if st.Releases[1].Name != "myrelease1" {
		t.Errorf("unexpected releases[1].name: expected=myrelease1, got=%s", st.Releases[1].Name)
	}
	if st.Releases[2].Name != "myrelease1" {
		t.Errorf("unexpected releases[2].name: expected=myrelease1, got=%s", st.Releases[2].Name)
	}
	if st.Releases[2].Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", st.Releases[1].Values[0])
	}
	if *st.Releases[2].MissingFileHandler != "Warn" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected=Warn, got=%s", *st.Releases[1].MissingFileHandler)
	}

	if st.Releases[2].Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", st.Releases[1].Values[0])
	}

	if st.HelmDefaults.KubeContext != "FOO" {
		t.Errorf("unexpected helmDefaults.kubeContext: expected=FOO, got=%s", st.HelmDefaults.KubeContext)
	}
}

func TestLoadDesiredStateFromYaml_MultiPartTemplate_WithReverse(t *testing.T) {
	yamlFile := "/path/to/yaml/file"
	yamlContent := `
{{ readFile "templates.yaml" }}

releases:
- name: myrelease0
  chart: mychart0
- name: myrelease1
  chart: mychart1
  <<: *default
---

{{ readFile "templates.yaml" }}

releases:
- name: myrelease2
  chart: mychart2
- name: myrelease3
  chart: mychart3
  <<: *default
`
	testFs := state.NewTestFs(map[string]string{
		yamlFile: yamlContent,
		"/path/to/yaml/templates.yaml": `templates:
  default: &default
    missingFileHandler: Warn
    values: ["` + "{{`" + `{{.Release.Name}}` + "`}}" + `/values.yaml"]
`,
	})
	app := &App{
		readFile: testFs.ReadFile,
		glob:     testFs.Glob,
		abs:      testFs.Abs,
		Env:      "default",
		Logger:   helmexec.NewLogger(os.Stderr, "debug"),
		Reverse:  true,
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.Releases[0].Name != "myrelease3" {
		t.Errorf("unexpected releases[0].name: expected=myrelease3, got=%s", st.Releases[0].Name)
	}
	if st.Releases[1].Name != "myrelease2" {
		t.Errorf("unexpected releases[0].name: expected=myrelease2, got=%s", st.Releases[1].Name)
	}
	if st.Releases[2].Name != "myrelease1" {
		t.Errorf("unexpected releases[0].name: expected=myrelease1, got=%s", st.Releases[2].Name)
	}
	if st.Releases[3].Name != "myrelease0" {
		t.Errorf("unexpected releases[0].name: expected=myrelease0, got=%s", st.Releases[3].Name)
	}
}
