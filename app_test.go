package main

import (
	"fmt"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"os"
	"testing"
)

// See https://github.com/roboll/helmfile/issues/193
func TestFindAndIterateOverDesiredStates(t *testing.T) {
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
	app := &app{
		readFile:          readFile,
		glob:              glob,
		abs:               abs,
		fileExistsAt:      fileExistsAt,
		directoryExistsAt: directoryExistsAt,
		kubeContext:       "default",
		logger:            helmexec.NewLogger(os.Stderr, "debug"),
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
		err := app.FindAndIterateOverDesiredStates(
			"helmfile.yaml", noop, "", []string{fmt.Sprintf("name=%s", testcase.name)}, "default",
		)
		if testcase.expectErr && err == nil {
			t.Errorf("error expected but not happened for name=%s", testcase.name)
		} else if !testcase.expectErr && err != nil {
			t.Errorf("unexpected error for name=%s: %v", testcase.name, err)
		}
	}
}
