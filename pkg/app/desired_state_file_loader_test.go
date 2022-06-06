package app

import (
	"testing"
)

func TestDesiredStateFileLoader(t *testing.T) {
	envName := "test"
	files := map[string]string{
		"/path/to/default/helmfile.yaml": `---
environments:
  test:
    values:
    - test-env.yaml.gotmpl
releases:
  - name: test-release
    chart: test-chart
`,
		"/path/to/default/test-env.yaml.gotmpl": `---
name: {{ .Environment.Name }}
`,
	}
	loader, _ := makeLoader(files, envName)

	state, err := loader.Load("/path/to/default/helmfile.yaml", LoadOpts{})
	if err != nil {
		t.Fatalf("cannot Load: %s", err)
	}
	if name, ok := state.Env.Values["name"]; ok {
		if name != envName {
			t.Errorf("name should be %s not %s", envName, name)
		}
	} else {
		t.Errorf("name is not defined")
	}
}
