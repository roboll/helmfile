package app

import (
	"fmt"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"

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
	app.fileExists = fs.FileExists
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

	expected := "in ./helmfile.yaml: failed to read helmfile.yaml: environment values file matching \"env.*.yaml\" does not exist in \".\""
	if err.Error() != expected {
		t.Errorf("unexpected error: expected=%s, got=%v", expected, err)
	}
}

func TestVisitDesiredStatesWithReleasesFiltered_MissingEnvValuesFileHandler(t *testing.T) {
	testcases := []struct {
		name        string
		handler     string
		filePattern string
		expectErr   bool
	}{
		{name: "error handler with no files matching glob", handler: "Error", filePattern: "env.*.yaml", expectErr: true},
		{name: "warn handler with no files matching glob", handler: "Warn", filePattern: "env.*.yaml", expectErr: false},
		{name: "info handler with no files matching glob", handler: "Info", filePattern: "env.*.yaml", expectErr: false},
		{name: "debug handler with no files matching glob", handler: "Debug", filePattern: "env.*.yaml", expectErr: false},
	}

	for i := range testcases {
		testcase := testcases[i]
		t.Run(testcase.name, func(t *testing.T) {
			files := map[string]string{
				"/path/to/helmfile.yaml": fmt.Sprintf(`
environments:
  default:
    missingFileHandler: %s
    values:
    - %s
releases:
- name: zipkin
  chart: stable/zipkin
`, testcase.handler, testcase.filePattern),
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
			if testcase.expectErr && err == nil {
				t.Fatal("expected error did not occur")
			}

			if !testcase.expectErr && err != nil {
				t.Errorf("not error expected, but got: %v", err)
			}
		})
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

func TestVisitDesiredStatesWithReleasesFiltered_EmbeddedNestedStateAdditionalEnvValues(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
helmfiles:
- path: helmfile.d/a*.yaml
  values:
  - env.values.yaml
- helmfile.d/b*.yaml
- path: helmfile.d/c*.yaml
  values:
  - env.values.yaml
  - tillerNs: INLINE_TILLER_NS_3
`,
		"/path/to/helmfile.d/a1.yaml": `
environments:
  default:
    values:
    - tillerNs: INLINE_TILLER_NS
      ns: INLINE_NS
releases:
- name: foo
  chart: stable/zipkin
  tillerNamespace: {{ .Environment.Values.tillerNs }}
  namespace: {{ .Environment.Values.ns }}
`,
		"/path/to/helmfile.d/b.yaml": `
environments:
  default:
    values:
    - tillerNs: INLINE_TILLER_NS
      ns: INLINE_NS
releases:
- name: bar
  chart: stable/grafana
  tillerNamespace:  {{ .Environment.Values.tillerNs }}
  namespace: {{ .Environment.Values.ns }}
`,
		"/path/to/helmfile.d/c.yaml": `
environments:
  default:
    values:
    - tillerNs: INLINE_TILLER_NS
      ns: INLINE_NS
releases:
- name: baz
  chart: stable/envoy
  tillerNamespace: {{ .Environment.Values.tillerNs }}
  namespace: {{ .Environment.Values.ns }}
`,
		"/path/to/env.values.yaml": `
tillerNs: INLINE_TILLER_NS_2
`,
	}

	app := appWithFs(&App{
		KubeContext: "default",
		Logger:      helmexec.NewLogger(os.Stderr, "debug"),
		Namespace:   "",
		Selectors:   []string{},
		Env:         "default",
	}, files)

	processed := []state.ReleaseSpec{}

	collectReleases := func(st *state.HelmState, helm helmexec.Interface) []error {
		for _, r := range st.Releases {
			processed = append(processed, r)
		}
		return []error{}
	}

	err := app.VisitDesiredStatesWithReleasesFiltered(
		"helmfile.yaml", collectReleases,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	type release struct {
		chart    string
		tillerNs string
		ns       string
	}

	expectedReleases := map[string]release{
		"foo": {"stable/zipkin", "INLINE_TILLER_NS_2", "INLINE_NS"},
		"bar": {"stable/grafana", "INLINE_TILLER_NS", "INLINE_NS"},
		"baz": {"stable/envoy", "INLINE_TILLER_NS_3", "INLINE_NS"},
	}

	for name := range processed {
		actual := processed[name]
		t.Run(actual.Name, func(t *testing.T) {
			expected, ok := expectedReleases[actual.Name]
			if !ok {
				t.Fatalf("unexpected release processed: %v", actual)
			}

			if expected.chart != actual.Chart {
				t.Errorf("unexpected chart: expected=%s, got=%s", expected.chart, actual.Chart)
			}

			if expected.tillerNs != actual.TillerNamespace {
				t.Errorf("unexpected tiller namespace: expected=%s, got=%s", expected.tillerNs, actual.TillerNamespace)
			}

			if expected.ns != actual.Namespace {
				t.Errorf("unexpected namespace: expected=%s, got=%s", expected.ns, actual.Namespace)
			}
		})
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

func TestVisitDesiredStatesWithReleasesFiltered_EnvironmentValueOverrides(t *testing.T) {
	files := map[string]string{
		"/path/to/helmfile.yaml": `
environments:
  default:
    values:
    - values.yaml
---
releases:
- name: {{ .Environment.Values.foo }}-{{ .Environment.Values.bar }}-{{ .Environment.Values.baz }}
  chart: stable/zipkin
`,
		"/path/to/values.yaml": `
foo: foo
bar: bar
baz: baz
`,
		"/path/to/overrides.yaml": `
foo: "foo1"
bar: "bar1"
`,
	}

	testcases := []struct {
		expected string
	}{
		{expected: "foo1-bar2-baz1"},
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
			Reverse:     false,
			Namespace:   "",
			Selectors:   []string{},
			Env:         "default",
			ValuesFiles: []string{"overrides.yaml"},
			Set:         map[string]interface{}{"bar": "bar2", "baz": "baz1"},
		}, files)
		err := app.VisitDesiredStatesWithReleasesFiltered(
			"helmfile.yaml", collectReleases,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actual) != 1 {
			t.Errorf("unexpected number of processed releases: expected=1, got=%d", len(actual))
		}
		if actual[0] != testcase.expected {
			t.Errorf("unexpected result: expected=%s, got=%s", testcase.expected, actual[0])
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
		readFile:     testFs.ReadFile,
		glob:         testFs.Glob,
		abs:          testFs.Abs,
		fileExistsAt: testFs.FileExistsAt,
		fileExists:   testFs.FileExists,
		KubeContext:  "default",
		Env:          "default",
		Logger:       helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		readFile:   testFs.ReadFile,
		fileExists: testFs.FileExists,
		glob:       testFs.Glob,
		abs:        testFs.Abs,
		Env:        "default",
		Logger:     helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if st.HelmDefaults.TillerNamespace != "TILLER_NS" {
		t.Errorf("unexpected helmDefaults.tillerNamespace: expected=TILLER_NS, got=%s", st.HelmDefaults.TillerNamespace)
	}
	firstRelease := st.Releases[0]
	if firstRelease.Name != "myrelease1" {
		t.Errorf("unexpected releases[1].name: expected=myrelease1, got=%s", firstRelease.Name)
	}
	secondRelease := st.Releases[1]
	if secondRelease.Name != "myrelease1" {
		t.Errorf("unexpected releases[2].name: expected=myrelease1, got=%s", secondRelease.Name)
	}
	if secondRelease.Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", firstRelease.Values[0])
	}
	if *secondRelease.MissingFileHandler != "Warn" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected=Warn, got=%s", *firstRelease.MissingFileHandler)
	}

	if secondRelease.Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", firstRelease.Values[0])
	}

	if st.HelmDefaults.KubeContext != "FOO" {
		t.Errorf("unexpected helmDefaults.kubeContext: expected=FOO, got=%s", st.HelmDefaults.KubeContext)
	}
}

func TestLoadDesiredStateFromYaml_EnvvalsInheritanceToBaseTemplate(t *testing.T) {
	yamlFile := "/path/to/yaml/file"
	yamlContent := `bases:
- ../base.yaml
---
bases:
# "envvals inheritance"
# base.gotmpl should be able to reference environment values defined in the base.yaml and default/1.yaml
- ../base.gotmpl
---
releases:
- name: myrelease0
  chart: mychart0
`
	testFs := state.NewTestFs(map[string]string{
		yamlFile: yamlContent,
		"/path/to/base.yaml": `environments:
  default:
    values:
    - environments/default/1.yaml
`,
		"/path/to/base.gotmpl": `helmDefaults:
  kubeContext: {{ .Environment.Values.foo }}
  tillerNamespace: {{ .Environment.Values.tillerNs }}
`,
		"/path/to/yaml/environments/default/1.yaml": `tillerNs: TILLER_NS
foo: FOO
`,
		"/path/to/yaml/templates.yaml": `templates:
  default: &default
    missingFileHandler: Warn
    values: ["` + "{{`" + `{{.Release.Name}}` + "`}}" + `/values.yaml"]
`,
	})
	app := &App{
		readFile:   testFs.ReadFile,
		fileExists: testFs.FileExists,
		glob:       testFs.Glob,
		abs:        testFs.Abs,
		Env:        "default",
		Logger:     helmexec.NewLogger(os.Stderr, "debug"),
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

	if st.HelmDefaults.KubeContext != "FOO" {
		t.Errorf("unexpected helmDefaults.kubeContext: expected=FOO, got=%s", st.HelmDefaults.KubeContext)
	}
}

func TestLoadDesiredStateFromYaml_InlineEnvVals(t *testing.T) {
	yamlFile := "/path/to/yaml/file"
	yamlContent := `bases:
- ../base.yaml
---
bases:
# "envvals inheritance"
# base.gotmpl should be able to reference environment values defined in the base.yaml and default/1.yaml
- ../base.gotmpl
---
releases:
- name: myrelease0
  chart: mychart0
`
	testFs := state.NewTestFs(map[string]string{
		yamlFile: yamlContent,
		"/path/to/base.yaml": `environments:
  default:
    values:
    - environments/default/1.yaml
    - tillerNs: INLINE_TILLER_NS
`,
		"/path/to/base.gotmpl": `helmDefaults:
  kubeContext: {{ .Environment.Values.foo }}
  tillerNamespace: {{ .Environment.Values.tillerNs }}
`,
		"/path/to/yaml/environments/default/1.yaml": `tillerNs: TILLER_NS
foo: FOO
`,
		"/path/to/yaml/templates.yaml": `templates:
  default: &default
    missingFileHandler: Warn
    values: ["` + "{{`" + `{{.Release.Name}}` + "`}}" + `/values.yaml"]
`,
	})
	app := &App{
		readFile:   testFs.ReadFile,
		fileExists: testFs.FileExists,
		glob:       testFs.Glob,
		abs:        testFs.Abs,
		Env:        "default",
		Logger:     helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if st.HelmDefaults.TillerNamespace != "INLINE_TILLER_NS" {
		t.Errorf("unexpected helmDefaults.tillerNamespace: expected=TILLER_NS, got=%s", st.HelmDefaults.TillerNamespace)
	}

	if st.Releases[0].Name != "myrelease0" {
		t.Errorf("unexpected releases[0].name: expected=myrelease0, got=%s", st.Releases[0].Name)
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
		readFile:   testFs.ReadFile,
		fileExists: testFs.FileExists,
		glob:       testFs.Glob,
		abs:        testFs.Abs,
		Env:        "test",
		Logger:     helmexec.NewLogger(os.Stderr, "debug"),
	}
	st, err := app.loadDesiredStateFromYaml(yamlFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.HelmDefaults.TillerNamespace != "TILLER_NS" {
		t.Errorf("unexpected helmDefaults.tillerNamespace: expected=TILLER_NS, got=%s", st.HelmDefaults.TillerNamespace)
	}

	firstRelease := st.Releases[0]
	if firstRelease.Name != "myrelease1" {
		t.Errorf("unexpected releases[1].name: expected=myrelease1, got=%s", firstRelease.Name)
	}
	secondRelease := st.Releases[1]
	if secondRelease.Name != "myrelease1" {
		t.Errorf("unexpected releases[2].name: expected=myrelease1, got=%s", secondRelease.Name)
	}
	if secondRelease.Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", firstRelease.Values[0])
	}
	if *secondRelease.MissingFileHandler != "Warn" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected=Warn, got=%s", *firstRelease.MissingFileHandler)
	}

	if secondRelease.Values[0] != "{{`{{.Release.Name}}`}}/values.yaml" {
		t.Errorf("unexpected releases[2].missingFileHandler: expected={{`{{.Release.Name}}`}}/values.yaml, got=%s", firstRelease.Values[0])
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

	if len(st.Releases) != 2 {
		t.Errorf("unexpected number of releases: expected=2, got=%d", len(st.Releases))
	}
}

// See https://github.com/roboll/helmfile/issues/615
func TestLoadDesiredStateFromYaml_MultiPartTemplate_NoMergeArrayInEnvVal(t *testing.T) {
	statePath := "/path/to/helmfile.yaml"
	stateContent := `
environments:
  default:
    values:
    - foo: ["foo"]
---
environments:
  default:
    values:
    - foo: ["FOO"]
    - 1.yaml
---
environments:
  default:
    values:
    - 2.yaml
---
releases:
- name: {{ .Environment.Values.foo | quote }}
  chart: {{ .Environment.Values.bar | quote }}
`
	testFs := state.NewTestFs(map[string]string{
		statePath:         stateContent,
		"/path/to/1.yaml": `bar: ["bar"]`,
		"/path/to/2.yaml": `bar: ["BAR"]`,
	})
	app := &App{
		readFile: testFs.ReadFile,
		glob:     testFs.Glob,
		abs:      testFs.Abs,
		Env:      "default",
		Logger:   helmexec.NewLogger(os.Stderr, "debug"),
		Reverse:  true,
	}
	st, err := app.loadDesiredStateFromYaml(statePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.Releases[0].Name != "[FOO]" {
		t.Errorf("unexpected releases[0].name: expected=FOO, got=%s", st.Releases[0].Name)
	}
	if st.Releases[0].Chart != "[BAR]" {
		t.Errorf("unexpected releases[0].chart: expected=BAR, got=%s", st.Releases[0].Chart)
	}
}

// See https://github.com/roboll/helmfile/issues/623
func TestLoadDesiredStateFromYaml_MultiPartTemplate_MergeMapsVariousKeys(t *testing.T) {
	type testcase struct {
		overrideValues interface{}
		expected       string
	}
	testcases := []testcase{
		{map[interface{}]interface{}{"foo": "FOO"}, `FOO`},
		{map[interface{}]interface{}{"foo": map[interface{}]interface{}{"foo": "FOO"}}, `map[foo:FOO]`},
		{map[interface{}]interface{}{"foo": map[string]interface{}{"foo": "FOO"}}, `map[foo:FOO]`},
		{map[interface{}]interface{}{"foo": []interface{}{"foo"}}, `[foo]`},
		{map[interface{}]interface{}{"foo": "FOO"}, `FOO`},
	}
	for i := range testcases {
		tc := testcases[i]
		statePath := "/path/to/helmfile.yaml"
		stateContent := `
environments:
  default:
    values:
    - 1.yaml
    - 2.yaml
---
releases:
- name: {{ .Environment.Values.foo | quote }}
  chart: {{ .Environment.Values.bar | quote }}
`
		testFs := state.NewTestFs(map[string]string{
			statePath:         stateContent,
			"/path/to/1.yaml": `bar: ["bar"]`,
			"/path/to/2.yaml": `bar: ["BAR"]`,
		})
		app := &App{
			readFile: testFs.ReadFile,
			glob:     testFs.Glob,
			abs:      testFs.Abs,
			Env:      "default",
			Logger:   helmexec.NewLogger(os.Stderr, "debug"),
			Reverse:  true,
		}
		opts := LoadOpts{
			CalleePath: statePath,
			Environment: state.SubhelmfileEnvironmentSpec{
				OverrideValues: []interface{}{tc.overrideValues},
			},
		}
		st, err := app.loadDesiredStateFromYaml(statePath, opts)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if st.Releases[0].Name != tc.expected {
			t.Errorf("unexpected releases[0].name: expected=%s, got=%s", tc.expected, st.Releases[0].Name)
		}
		if st.Releases[0].Chart != "[BAR]" {
			t.Errorf("unexpected releases[0].chart: expected=BAR, got=%s", st.Releases[0].Chart)
		}
	}
}

func TestLoadDesiredStateFromYaml_MultiPartTemplate_SprigDictFuncs(t *testing.T) {
	type testcase struct {
		state    string
		expr     string
		expected string
	}
	stateInline := `
environments:
  default:
    values:
    - foo: FOO
      bar: { "baz": "BAZ" }
---
releases:
- name: %s
  chart: stable/nginx
`
	stateExternal := `
environments:
  default:
    values:
    - 1.yaml
    - 2.yaml
---
releases:
- name: %s
  chart: stable/nginx
`
	testcases := []testcase{
		{stateInline, `{{ getOrNil "foo" .Environment.Values }}`, `FOO`},
		{stateInline, `{{ getOrNil "baz" (getOrNil "bar" .Environment.Values) }}`, `BAZ`},
		{stateInline, `{{ if hasKey .Environment.Values "foo" }}{{ .Environment.Values.foo }}{{ end }}`, `FOO`},
		{stateInline, `{{ if hasKey .Environment.Values "bar" }}{{ .Environment.Values.bar.baz }}{{ end }}`, `BAZ`},
		{stateInline, `{{ if (keys .Environment.Values | has "foo") }}{{ .Environment.Values.foo }}{{ end }}`, `FOO`},
		// See https://github.com/roboll/helmfile/issues/624
		// This fails when .Environment.Values.bar is not map[string]interface{}. At the time of #624 it was map[interface{}]interface{}, which sprig's dict funcs don't support.
		{stateInline, `{{ if (keys .Environment.Values | has "bar") }}{{ if (keys .Environment.Values.bar | has "baz") }}{{ .Environment.Values.bar.baz }}{{ end }}{{ end }}`, `BAZ`},
		{stateExternal, `{{ getOrNil "foo" .Environment.Values }}`, `FOO`},
		{stateExternal, `{{ getOrNil "baz" (getOrNil "bar" .Environment.Values) }}`, `BAZ`},
		{stateExternal, `{{ if hasKey .Environment.Values "foo" }}{{ .Environment.Values.foo }}{{ end }}`, `FOO`},
		{stateExternal, `{{ if hasKey .Environment.Values "bar" }}{{ .Environment.Values.bar.baz }}{{ end }}`, `BAZ`},
		{stateExternal, `{{ if (keys .Environment.Values | has "foo") }}{{ .Environment.Values.foo }}{{ end }}`, `FOO`},
		// See https://github.com/roboll/helmfile/issues/624
		{stateExternal, `{{ if (keys .Environment.Values | has "bar") }}{{ if (keys .Environment.Values.bar | has "baz") }}{{ .Environment.Values.bar.baz }}{{ end }}{{ end }}`, `BAZ`},
		// See https://github.com/roboll/helmfile/issues/643
		{stateExternal, `{{ range $service := .Environment.Values.services }}{{ $service.name }}{{ if hasKey $service "something" }}{{ $service.something }}{{ end }}{{ end }}`, `xyfalse`},
	}
	for i := range testcases {
		tc := testcases[i]
		statePath := "/path/to/helmfile.yaml"
		stateContent := fmt.Sprintf(tc.state, tc.expr)
		testFs := state.NewTestFs(map[string]string{
			statePath:         stateContent,
			"/path/to/1.yaml": `foo: FOO`,
			"/path/to/2.yaml": `bar: { "baz": "BAZ" }
services:
  - name: "x"
  - name: "y"
    something: false
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
		st, err := app.loadDesiredStateFromYaml(statePath)

		if err != nil {
			t.Fatalf("unexpected error at %d: %v", i, err)
		}

		if st.Releases[0].Name != tc.expected {
			t.Errorf("unexpected releases[0].name at %d: expected=%s, got=%s", i, tc.expected, st.Releases[0].Name)
		}
	}
}
