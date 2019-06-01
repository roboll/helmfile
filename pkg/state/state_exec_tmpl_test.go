package state

import (
	"github.com/roboll/helmfile/pkg/environment"
	"reflect"
	"testing"
)

func TestHelmState_executeTemplates(t *testing.T) {
	tests := []struct {
		name  string
		input ReleaseSpec
		want  ReleaseSpec
	}{
		{
			name: "Has template expressions in chart, values, and secrets",
			input: ReleaseSpec{
				Chart:     "test-charts/{{ .Release.Name }}",
				Version:   "0.1",
				Verify:    nil,
				Name:      "test-app",
				Namespace: "test-namespace-{{ .Release.Name }}",
				Values:    []interface{}{"config/{{ .Environment.Name }}/{{ .Release.Name }}/values.yaml"},
				Secrets:   []string{"config/{{ .Environment.Name }}/{{ .Release.Name }}/secrets.yaml"},
			},
			want: ReleaseSpec{
				Chart:     "test-charts/test-app",
				Version:   "0.1",
				Verify:    nil,
				Name:      "test-app",
				Namespace: "test-namespace-test-app",
				Values:    []interface{}{"config/test_env/test-app/values.yaml"},
				Secrets:   []string{"config/test_env/test-app/secrets.yaml"},
			},
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				basePath: ".",
				HelmDefaults: HelmSpec{
					KubeContext: "test_context",
				},
				Env:          environment.Environment{Name: "test_env"},
				Namespace:    "test-namespace_",
				Repositories: nil,
				Releases: []ReleaseSpec{
					tt.input,
				},
			}

			r, err := state.ExecuteTemplates()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				t.FailNow()
			}

			actual := r.Releases[0]

			if !reflect.DeepEqual(actual.Chart, tt.want.Chart) {
				t.Errorf("expected %+v, got %+v", tt.want.Chart, actual.Chart)
			}
			if !reflect.DeepEqual(actual.Namespace, tt.want.Namespace) {
				t.Errorf("expected %+v, got %+v", tt.want.Namespace, actual.Namespace)
			}
			if !reflect.DeepEqual(actual.Values, tt.want.Values) {
				t.Errorf("expected %+v, got %+v", tt.want.Values, actual.Values)
			}
			if !reflect.DeepEqual(actual.Secrets, tt.want.Secrets) {
				t.Errorf("expected %+v, got %+v", tt.want.Secrets, actual.Secrets)
			}
		})
	}
}
