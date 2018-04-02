package state

import (
	"os"
	"reflect"
	"testing"

	"errors"
	"strings"
)

func TestReadFromYaml(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease
  namespace: mynamespace
  chart: mychart
`)
	state, err := readFromYaml(yamlContent, yamlFile)
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

func TestReadFromYaml_DeprecatedReleaseReferences(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`charts:
- name: myrelease
  chart: mychart
`)
	state, err := readFromYaml(yamlContent, yamlFile)
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
	_, err := readFromYaml(yamlContent, yamlFile)
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
		{LabelFilter{positiveLabels: map[string]string{"tier": "frontend"}},
			[]bool{true, true, false}},
		{LabelFilter{positiveLabels: map[string]string{"tier": "frontend", "foo": "bar"}},
			[]bool{true, false, false}},
		{LabelFilter{negativeLabels: map[string]string{"tier": "frontend"}},
			[]bool{false, false, true}},
		{LabelFilter{positiveLabels: map[string]string{"tier": "frontend"}, negativeLabels: map[string]string{"foo": "bar"}},
			[]bool{false, true, false}},
	}
	state, err := readFromYaml(yamlContent, yamlFile)
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

func TestLabelParsing(t *testing.T) {
	cases := []struct {
		labelString    string
		expectedFilter LabelFilter
		errorExected   bool
	}{
		{"foo=bar", LabelFilter{positiveLabels: map[string]string{"foo": "bar"}, negativeLabels: map[string]string{}}, false},
		{"foo!=bar", LabelFilter{positiveLabels: map[string]string{}, negativeLabels: map[string]string{"foo": "bar"}}, false},
		{"foo!=bar,baz=bat", LabelFilter{positiveLabels: map[string]string{"baz": "bat"}, negativeLabels: map[string]string{"foo": "bar"}}, false},
		{"foo", LabelFilter{positiveLabels: map[string]string{}, negativeLabels: map[string]string{}}, true},
		{"foo!=bar=baz", LabelFilter{positiveLabels: map[string]string{}, negativeLabels: map[string]string{}}, true},
		{"=bar", LabelFilter{positiveLabels: map[string]string{}, negativeLabels: map[string]string{}}, true},
	}
	for idx, c := range cases {
		filter, err := ParseLabels(c.labelString)
		if err != nil && !c.errorExected {
			t.Errorf("[%d] Didn't expect an error parsing labels: %s", idx, err)
		} else if err == nil && c.errorExected {
			t.Errorf("[%d] Expected %s to result in an error but got none", idx, c.labelString)
		} else if !reflect.DeepEqual(filter, c.expectedFilter) {
			t.Errorf("[%d] parsed label did not result in expected filter: %v", idx, filter)
		}
	}
}
func TestHelmState_applyDefaultsTo(t *testing.T) {
	type fields struct {
		BaseChartPath      string
		Context            string
		DeprecatedReleases []ReleaseSpec
		Namespace          string
		Repositories       []RepositorySpec
		Releases           []ReleaseSpec
	}
	type args struct {
		spec ReleaseSpec
	}
	specWithNamespace := ReleaseSpec{
		Chart:     "test/chart",
		Version:   "0.1",
		Verify:    false,
		Name:      "test-charts",
		Namespace: "test-namespace",
		Values:    nil,
		SetValues: nil,
		EnvValues: nil,
	}

	specWithoutNamespace := specWithNamespace
	specWithoutNamespace.Namespace = ""
	specWithNamespaceFromFields := specWithNamespace
	specWithNamespaceFromFields.Namespace = "test-namespace-field"

	fieldsWithNamespace := fields{
		BaseChartPath:      ".",
		Context:            "test_context",
		DeprecatedReleases: nil,
		Namespace:          specWithNamespaceFromFields.Namespace,
		Repositories:       nil,
		Releases: []ReleaseSpec{
			specWithNamespace,
		},
	}

	fieldsWithoutNamespace := fieldsWithNamespace
	fieldsWithoutNamespace.Namespace = ""

	tests := []struct {
		name   string
		fields fields
		args   args
		want   ReleaseSpec
	}{
		{
			name:   "Has a namespace from spec",
			fields: fieldsWithoutNamespace,
			args: args{
				spec: specWithNamespace,
			},
			want: specWithNamespace,
		},
		{
			name:   "Has a namespace from flags",
			fields: fieldsWithoutNamespace,
			args: args{
				spec: specWithNamespace,
			},
			want: specWithNamespace,
		},
		{
			name:   "Has a namespace from flags and from spec",
			fields: fieldsWithNamespace,
			args: args{
				spec: specWithNamespace,
			},
			want: specWithNamespaceFromFields,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				BaseChartPath:      tt.fields.BaseChartPath,
				Context:            tt.fields.Context,
				DeprecatedReleases: tt.fields.DeprecatedReleases,
				Namespace:          tt.fields.Namespace,
				Repositories:       tt.fields.Repositories,
				Releases:           tt.fields.Releases,
			}
			if got := state.applyDefaultsTo(tt.args.spec); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HelmState.applyDefaultsTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_renderTemplateString(t *testing.T) {
	type args struct {
		s    string
		envs map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "simple replacement",
			args: args{
				s: "{{ env \"HF_TEST_VAR\" }}",
				envs: map[string]string{
					"HF_TEST_VAR": "content",
				},
			},
			want:    "content",
			wantErr: false,
		},
		{
			name: "two replacements",
			args: args{
				s: "{{ env \"HF_TEST_ALPHA\" }}{{ env \"HF_TEST_BETA\" }}",
				envs: map[string]string{
					"HF_TEST_ALPHA": "first",
					"HF_TEST_BETA":  "second",
				},
			},
			want:    "firstsecond",
			wantErr: false,
		},
		{
			name: "replacement and comment",
			args: args{
				s: "{{ env \"HF_TEST_ALPHA\" }}{{/* comment */}}",
				envs: map[string]string{
					"HF_TEST_ALPHA": "first",
				},
			},
			want:    "first",
			wantErr: false,
		},
		{
			name: "global template function",
			args: args{
				s: "{{ env \"HF_TEST_ALPHA\" | len }}",
				envs: map[string]string{
					"HF_TEST_ALPHA": "abcdefg",
				},
			},
			want:    "7",
			wantErr: false,
		},
		{
			name: "env var not set",
			args: args{
				s: "{{ env \"HF_TEST_NONE\" }}",
				envs: map[string]string{
					"HF_TEST_THIS": "first",
				},
			},
			wantErr: true,
		},
		{
			name: "undefined function",
			args: args{
				s: "{{ env foo }}",
				envs: map[string]string{
					"foo": "bar",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.args.envs {
				err := os.Setenv(k, v)
				if err != nil {
					t.Error("renderTemplateString() could not set env var for testing")
				}
			}
			got, err := renderTemplateString(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderTemplateString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("renderTemplateString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isLocalChart(t *testing.T) {
	type args struct {
		chart string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "local chart",
			args: args{
				chart: "./charts/nonstop",
			},
			want: true,
		},
		{
			name: "repo chart",
			args: args{
				chart: "stable/genius",
			},
			want: false,
		},
		{
			name: "empty",
			args: args{
				chart: "",
			},
			want: false,
		},
		{
			name: "parent local path",
			args: args{
				chart: "../../dotty",
			},
			want: true,
		},
		{
			name: "parent-parent local path",
			args: args{
				chart: "../../dotty",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLocalChart(tt.args.chart); got != tt.want {
				t.Errorf("isLocalChart() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_normalizeChart(t *testing.T) {
	type args struct {
		basePath string
		chart    string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "construct local chart path",
			args: args{
				basePath: "/Users/jane/code/deploy/charts",
				chart:    "./app",
			},
			want: "/Users/jane/code/deploy/charts/app",
		},
		{
			name: "repo path",
			args: args{
				basePath: "/Users/jane/code/deploy/charts",
				chart:    "remote/app",
			},
			want: "remote/app",
		},
		{
			name: "construct local chart path, parent dir",
			args: args{
				basePath: "/Users/jane/code/deploy/charts",
				chart:    "../app",
			},
			want: "/Users/jane/code/deploy/app",
		},
		{
			name: "too much parent levels",
			args: args{
				basePath: "/src",
				chart:    "../../app",
			},
			want: "/app",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeChart(tt.args.basePath, tt.args.chart); got != tt.want {
				t.Errorf("normalizeChart() = %v, want %v", got, tt.want)
			}
		})
	}
}

// mocking helmexec.Interface

type mockHelmExec struct {
	charts []string
	repo   []string
}

func (helm *mockHelmExec) UpdateDeps(chart string) error {
	if strings.Contains(chart, "error") {
		return errors.New("error")
	}
	helm.charts = append(helm.charts, chart)
	return nil
}

func (helm *mockHelmExec) SetExtraArgs(args ...string) {
	return
}
func (helm *mockHelmExec) AddRepo(name, repository, certfile, keyfile string) error {
	helm.repo = []string{name, repository, certfile, keyfile}
	return nil
}
func (helm *mockHelmExec) UpdateRepo() error {
	return nil
}
func (helm *mockHelmExec) SyncRelease(name, chart string, flags ...string) error {
	return nil
}
func (helm *mockHelmExec) DiffRelease(name, chart string, flags ...string) error {
	return nil
}
func (helm *mockHelmExec) DeleteRelease(name string) error {
	return nil
}
func (helm *mockHelmExec) DecryptSecret(name string) (string, error) {
	return "", nil
}

func TestHelmState_SyncRepos(t *testing.T) {
	tests := []struct {
		name  string
		repos []RepositorySpec
		helm  *mockHelmExec
		envs  map[string]string
		want  []string
	}{
		{
			name: "normal repository",
			repos: []RepositorySpec{
				{
					Name:     "name",
					URL:      "http://example.com/",
					CertFile: "",
					KeyFile:  "",
				},
			},
			helm: &mockHelmExec{},
			want: []string{"name", "http://example.com/", "", ""},
		},
		{
			name: "repository with cert and key",
			repos: []RepositorySpec{
				{
					Name:     "name",
					URL:      "http://example.com/",
					CertFile: "certfile",
					KeyFile:  "keyfile",
				},
			},
			helm: &mockHelmExec{},
			want: []string{"name", "http://example.com/", "certfile", "keyfile"},
		},
		{
			name: "repository with env var url",
			repos: []RepositorySpec{
				{
					Name:     "name",
					URL:      "https://{{ env \"HF_TEST_GITHUB_TOKEN\"}}@raw.githubusercontent.com/u/r/b/",
					CertFile: "certfile",
					KeyFile:  "keyfile",
				},
			},
			envs: map[string]string{
				"HF_TEST_GITHUB_TOKEN": "token",
			},
			helm: &mockHelmExec{},
			want: []string{"name", "https://token@raw.githubusercontent.com/u/r/b/", "certfile", "keyfile"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envs {
				err := os.Setenv(k, v)
				if err != nil {
					t.Error("HelmState.SyncRepos() could not set env var for testing")
				}
			}
			state := &HelmState{
				Repositories: tt.repos,
			}
			if got := state.SyncRepos(tt.helm); !reflect.DeepEqual(tt.helm.repo, tt.want) {
				t.Errorf("HelmState.SyncRepos() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelmState_UpdateDeps(t *testing.T) {
	state := &HelmState{
		BaseChartPath: "/src",
		Releases: []ReleaseSpec{
			{
				Chart: "./local",
			},
			{
				Chart: "../local",
			},
			{
				Chart: "../../local",
			},
			{
				Chart: "published",
			},
			{
				Chart: "published/deeper",
			},
			{
				Chart: "./error",
			},
		},
	}
	want := []string{"/src/local", "/local", "/local"}
	helm := &mockHelmExec{}
	errs := state.UpdateDeps(helm)
	if !reflect.DeepEqual(helm.charts, want) {
		t.Errorf("HelmState.UpdateDeps() = %v, want %v", helm.charts, want)
	}
	if len(errs) != 1 {
		t.Errorf("HelmState.UpdateDeps() - expected an error, but got: %v", len(errs))
	}
}
