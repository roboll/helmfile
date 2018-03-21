package state

import (
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

func TestReadFromYaml_FilterReleasesOnTags(t *testing.T) {
	yamlFile := "example/path/to/yaml/file"
	yamlContent := []byte(`releases:
- name: myrelease1
  chart: mychart1
  tags:
    tier: frontend
    foo: bar
- name: myrelease2
  chart: mychart2
  tags:
    tier: frontend
- name: myrelease3
  chart: mychart3
  tags:
    tier: backend
`)
	cases := []struct {
		filter  TagFilter
		results []bool
	}{
		{TagFilter{positiveTags: map[string]string{"tier": "frontend"}},
			[]bool{true, true, false}},
		{TagFilter{positiveTags: map[string]string{"tier": "frontend", "foo": "bar"}},
			[]bool{true, false, false}},
		{TagFilter{negativeTags: map[string]string{"tier": "frontend"}},
			[]bool{false, false, true}},
		{TagFilter{positiveTags: map[string]string{"tier": "frontend"}, negativeTags: map[string]string{"foo": "bar"}},
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

func TestTagParsing(t *testing.T) {
	cases := []struct {
		tagString      string
		expectedFilter TagFilter
		errorExected   bool
	}{
		{"foo=bar", TagFilter{positiveTags: map[string]string{"foo": "bar"}, negativeTags: map[string]string{}}, false},
		{"foo!=bar", TagFilter{positiveTags: map[string]string{}, negativeTags: map[string]string{"foo": "bar"}}, false},
		{"foo!=bar,baz=bat", TagFilter{positiveTags: map[string]string{"baz": "bat"}, negativeTags: map[string]string{"foo": "bar"}}, false},
		{"foo", TagFilter{positiveTags: map[string]string{}, negativeTags: map[string]string{}}, true},
		{"foo!=bar=baz", TagFilter{positiveTags: map[string]string{}, negativeTags: map[string]string{}}, true},
		{"=bar", TagFilter{positiveTags: map[string]string{}, negativeTags: map[string]string{}}, true},
	}
	for idx, c := range cases {
		filter, err := ParseTags(c.tagString)
		if err != nil && !c.errorExected {
			t.Errorf("[%d] Didn't expect an error parsing tags: %s", idx, err)
		} else if err == nil && c.errorExected {
			t.Errorf("[%d] Expected %s to result in an error but got none", idx, c.tagString)
		} else if !reflect.DeepEqual(filter, c.expectedFilter) {
			t.Errorf("[%d] parsed tag did not result in expected filter: %v", idx, filter)
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
