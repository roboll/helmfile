package state

import (
	"os"
	"reflect"
	"testing"

	"errors"
	"strings"

	"fmt"
	"github.com/roboll/helmfile/helmexec"
)

var logger = helmexec.NewLogger(os.Stdout, "warn")

func TestLabelParsing(t *testing.T) {
	cases := []struct {
		labelString    string
		expectedFilter LabelFilter
		errorExected   bool
	}{
		{"foo=bar", LabelFilter{positiveLabels: [][]string{[]string{"foo", "bar"}}, negativeLabels: [][]string{}}, false},
		{"foo!=bar", LabelFilter{positiveLabels: [][]string{}, negativeLabels: [][]string{[]string{"foo", "bar"}}}, false},
		{"foo!=bar,baz=bat", LabelFilter{positiveLabels: [][]string{[]string{"baz", "bat"}}, negativeLabels: [][]string{[]string{"foo", "bar"}}}, false},
		{"foo", LabelFilter{positiveLabels: [][]string{}, negativeLabels: [][]string{}}, true},
		{"foo!=bar=baz", LabelFilter{positiveLabels: [][]string{}, negativeLabels: [][]string{}}, true},
		{"=bar", LabelFilter{positiveLabels: [][]string{}, negativeLabels: [][]string{}}, true},
	}
	for idx, c := range cases {
		filter, err := ParseLabels(c.labelString)
		if err != nil && !c.errorExected {
			t.Errorf("[%d] Didn't expect an error parsing labels: %s", idx, err)
		} else if err == nil && c.errorExected {
			t.Errorf("[%d] Expected %s to result in an error but got none", idx, c.labelString)
		} else if !reflect.DeepEqual(filter, c.expectedFilter) {
			t.Errorf("[%d] parsed label did not result in expected filter: %v, expected: %v", idx, filter, c.expectedFilter)
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
	verify := false
	specWithNamespace := ReleaseSpec{
		Chart:     "test/chart",
		Version:   "0.1",
		Verify:    &verify,
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
				basePath:           tt.fields.BaseChartPath,
				DeprecatedContext:  tt.fields.Context,
				DeprecatedReleases: tt.fields.DeprecatedReleases,
				Namespace:          tt.fields.Namespace,
				Repositories:       tt.fields.Repositories,
				Releases:           tt.fields.Releases,
			}
			if state.applyDefaultsTo(&tt.args.spec); !reflect.DeepEqual(tt.args.spec, tt.want) {
				t.Errorf("HelmState.applyDefaultsTo() = %v, want %v", tt.args.spec, tt.want)
			}
		})
	}
}

func boolValue(v bool) *bool {
	return &v
}

func TestHelmState_flagsForUpgrade(t *testing.T) {
	enable := true
	disable := false

	some := func(v int) *int {
		return &v
	}

	tests := []struct {
		name     string
		defaults HelmSpec
		release  *ReleaseSpec
		want     []string
	}{
		{
			name: "no-options",
			defaults: HelmSpec{
				Verify: false,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Verify:    &disable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "verify",
			defaults: HelmSpec{
				Verify: false,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Verify:    &enable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--verify",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "verify-from-default",
			defaults: HelmSpec{
				Verify: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Verify:    &disable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "force",
			defaults: HelmSpec{
				Force: false,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Force:     &enable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--force",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "force-from-default",
			defaults: HelmSpec{
				Force: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Force:     &disable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "recreate-pods",
			defaults: HelmSpec{
				RecreatePods: false,
			},
			release: &ReleaseSpec{
				Chart:        "test/chart",
				Version:      "0.1",
				RecreatePods: &enable,
				Name:         "test-charts",
				Namespace:    "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--recreate-pods",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "recreate-pods-from-default",
			defaults: HelmSpec{
				RecreatePods: true,
			},
			release: &ReleaseSpec{
				Chart:        "test/chart",
				Version:      "0.1",
				RecreatePods: &disable,
				Name:         "test-charts",
				Namespace:    "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "wait",
			defaults: HelmSpec{
				Wait: false,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Wait:      &enable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--wait",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "devel",
			defaults: HelmSpec{
				Devel: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Wait:      &enable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--devel",
				"--wait",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "devel-release",
			defaults: HelmSpec{
				Devel: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Devel:     &disable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "wait-from-default",
			defaults: HelmSpec{
				Wait: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Wait:      &disable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "timeout",
			defaults: HelmSpec{
				Timeout: 0,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Timeout:   some(123),
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--timeout", "123",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "timeout-from-default",
			defaults: HelmSpec{
				Timeout: 123,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Timeout:   nil,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--timeout", "123",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "atomic",
			defaults: HelmSpec{
				Atomic: false,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Atomic:    &enable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--atomic",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "atomic-override-default",
			defaults: HelmSpec{
				Atomic: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Atomic:    &disable,
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--namespace", "test-namespace",
			},
		},
		{
			name: "atomic-from-default",
			defaults: HelmSpec{
				Atomic: true,
			},
			release: &ReleaseSpec{
				Chart:     "test/chart",
				Version:   "0.1",
				Name:      "test-charts",
				Namespace: "test-namespace",
			},
			want: []string{
				"--version", "0.1",
				"--atomic",
				"--namespace", "test-namespace",
			},
		},
		{
			name:     "tiller",
			defaults: HelmSpec{},
			release: &ReleaseSpec{
				Chart:           "test/chart",
				Version:         "0.1",
				Name:            "test-charts",
				TLS:             boolValue(true),
				TillerNamespace: "tiller-system",
				TLSKey:          "key.pem",
				TLSCert:         "cert.pem",
				TLSCACert:       "ca.pem",
			},
			want: []string{
				"--version", "0.1",
				"--tiller-namespace", "tiller-system",
				"--tls",
				"--tls-key", "key.pem",
				"--tls-cert", "cert.pem",
				"--tls-ca-cert", "ca.pem",
			},
		},
		{
			name: "tiller-override-defaults",
			defaults: HelmSpec{
				TLS:             false,
				TillerNamespace: "a",
				TLSKey:          "b.pem",
				TLSCert:         "c.pem",
				TLSCACert:       "d.pem",
			},
			release: &ReleaseSpec{
				Chart:           "test/chart",
				Version:         "0.1",
				Name:            "test-charts",
				TLS:             boolValue(true),
				TillerNamespace: "tiller-system",
				TLSKey:          "key.pem",
				TLSCert:         "cert.pem",
				TLSCACert:       "ca.pem",
			},
			want: []string{
				"--version", "0.1",
				"--tiller-namespace", "tiller-system",
				"--tls",
				"--tls-key", "key.pem",
				"--tls-cert", "cert.pem",
				"--tls-ca-cert", "ca.pem",
			},
		},
		{
			name: "tiller-from-defaults",
			defaults: HelmSpec{
				TLS:             true,
				TillerNamespace: "tiller-system",
				TLSKey:          "key.pem",
				TLSCert:         "cert.pem",
				TLSCACert:       "ca.pem",
			},
			release: &ReleaseSpec{
				Chart:   "test/chart",
				Version: "0.1",
				Name:    "test-charts",
			},
			want: []string{
				"--version", "0.1",
				"--tiller-namespace", "tiller-system",
				"--tls",
				"--tls-key", "key.pem",
				"--tls-cert", "cert.pem",
				"--tls-ca-cert", "ca.pem",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				basePath:          "./",
				DeprecatedContext: "default",
				Releases:          []ReleaseSpec{*tt.release},
				HelmDefaults:      tt.defaults,
			}
			helm := helmexec.New(logger, "default")
			args, err := state.flagsForUpgrade(helm, tt.release)
			if err != nil {
				t.Errorf("unexpected error flagsForUpgade: %v", err)
			}
			if !reflect.DeepEqual(args, tt.want) {
				t.Errorf("flagsForUpgrade returned = %v, want %v", args, tt.want)
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
				chart: "./",
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
				chart: "../examples",
			},
			want: true,
		},
		{
			name: "parent-parent local path",
			args: args{
				chart: "../../",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLocalChart(tt.args.chart); got != tt.want {
				t.Errorf("pathExists() = %v, want %v", got, tt.want)
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

type listKey struct {
	filter string
	flags  string
}

type mockHelmExec struct {
	charts   []string
	repo     []string
	releases []mockRelease
	deleted  []mockRelease
	lists    map[listKey]string
	diffed   []mockRelease
}

type mockRelease struct {
	name  string
	flags []string
}

func (helm *mockHelmExec) UpdateDeps(chart string) error {
	if strings.Contains(chart, "error") {
		return errors.New("error")
	}
	helm.charts = append(helm.charts, chart)
	return nil
}

func (helm *mockHelmExec) BuildDeps(chart string) error {
	if strings.Contains(chart, "error") {
		return errors.New("error")
	}
	helm.charts = append(helm.charts, chart)
	return nil
}

func (helm *mockHelmExec) SetExtraArgs(args ...string) {
	return
}
func (helm *mockHelmExec) SetHelmBinary(bin string) {
	return
}
func (helm *mockHelmExec) AddRepo(name, repository, certfile, keyfile, username, password string) error {
	helm.repo = []string{name, repository, certfile, keyfile, username, password}
	return nil
}
func (helm *mockHelmExec) UpdateRepo() error {
	return nil
}
func (helm *mockHelmExec) SyncRelease(context helmexec.HelmContext, name, chart string, flags ...string) error {
	if strings.Contains(name, "error") {
		return errors.New("error")
	}
	helm.releases = append(helm.releases, mockRelease{name: name, flags: flags})
	helm.charts = append(helm.charts, chart)
	return nil
}
func (helm *mockHelmExec) DiffRelease(context helmexec.HelmContext, name, chart string, flags ...string) error {
	helm.diffed = append(helm.diffed, mockRelease{name: name, flags: flags})
	return nil
}
func (helm *mockHelmExec) ReleaseStatus(release string, flags ...string) error {
	if strings.Contains(release, "error") {
		return errors.New("error")
	}
	helm.releases = append(helm.releases, mockRelease{name: release, flags: flags})
	return nil
}
func (helm *mockHelmExec) DeleteRelease(context helmexec.HelmContext, name string, flags ...string) error {
	helm.deleted = append(helm.deleted, mockRelease{name: name, flags: flags})
	return nil
}
func (helm *mockHelmExec) List(context helmexec.HelmContext, filter string, flags ...string) (string, error) {
	return helm.lists[listKey{filter: filter, flags: strings.Join(flags, "")}], nil
}
func (helm *mockHelmExec) DecryptSecret(context helmexec.HelmContext, name string, flags ...string) (string, error) {
	return "", nil
}
func (helm *mockHelmExec) TestRelease(context helmexec.HelmContext, name string, flags ...string) error {
	if strings.Contains(name, "error") {
		return errors.New("error")
	}
	helm.releases = append(helm.releases, mockRelease{name: name, flags: flags})
	return nil
}
func (helm *mockHelmExec) Fetch(chart string, flags ...string) error {
	return nil
}
func (helm *mockHelmExec) Lint(chart string, flags ...string) error {
	return nil
}
func (helm *mockHelmExec) TemplateRelease(chart string, flags ...string) error {
	return nil
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
					Username: "",
					Password: "",
				},
			},
			helm: &mockHelmExec{},
			want: []string{"name", "http://example.com/", "", "", "", ""},
		},
		{
			name: "repository with cert and key",
			repos: []RepositorySpec{
				{
					Name:     "name",
					URL:      "http://example.com/",
					CertFile: "certfile",
					KeyFile:  "keyfile",
					Username: "",
					Password: "",
				},
			},
			helm: &mockHelmExec{},
			want: []string{"name", "http://example.com/", "certfile", "keyfile", "", ""},
		},
		{
			name: "repository with username and password",
			repos: []RepositorySpec{
				{
					Name:     "name",
					URL:      "http://example.com/",
					CertFile: "",
					KeyFile:  "",
					Username: "example_user",
					Password: "example_password",
				},
			},
			helm: &mockHelmExec{},
			want: []string{"name", "http://example.com/", "", "", "example_user", "example_password"},
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
			if _ = state.SyncRepos(tt.helm); !reflect.DeepEqual(tt.helm.repo, tt.want) {
				t.Errorf("HelmState.SyncRepos() for [%s] = %v, want %v", tt.name, tt.helm.repo, tt.want)
			}
		})
	}
}

func TestHelmState_SyncReleases(t *testing.T) {
	tests := []struct {
		name         string
		releases     []ReleaseSpec
		helm         *mockHelmExec
		wantReleases []mockRelease
	}{
		{
			name: "normal release",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{}}},
		},
		{
			name: "with tiller args",
			releases: []ReleaseSpec{
				{
					Name:            "releaseName",
					Chart:           "foo",
					TillerNamespace: "tillerns",
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--tiller-namespace", "tillerns"}}},
		},
		{
			name: "escaped values",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					SetValues: []SetValue{
						{
							Name:  "someList",
							Value: "a,b,c",
						},
						{
							Name:  "json",
							Value: "{\"name\": \"john\"}",
						},
					},
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--set", "someList=a\\,b\\,c", "--set", "json=\\{\"name\": \"john\"\\}"}}},
		},
		{
			name: "set single value from file",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					SetValues: []SetValue{
						{
							Name:  "foo",
							Value: "FOO",
						},
						{
							Name: "bar",
							File: "path/to/bar",
						},
						{
							Name:  "baz",
							Value: "BAZ",
						},
					},
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--set", "foo=FOO", "--set-file", "bar=path/to/bar", "--set", "baz=BAZ"}}},
		},
		{
			name: "set single array value in an array",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					SetValues: []SetValue{
						{
							Name: "foo.bar[0]",
							Values: []string{
								"A",
								"B",
							},
						},
					},
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--set", "foo.bar[0]={A,B}"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
			}
			if _ = state.SyncReleases(tt.helm, []string{}, 1); !reflect.DeepEqual(tt.helm.releases, tt.wantReleases) {
				t.Errorf("HelmState.SyncReleases() for [%s] = %v, want %v", tt.name, tt.helm.releases, tt.wantReleases)
			}
		})
	}
}

func TestHelmState_DiffReleases(t *testing.T) {
	tests := []struct {
		name         string
		releases     []ReleaseSpec
		helm         *mockHelmExec
		wantReleases []mockRelease
	}{
		{
			name: "normal release",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{}}},
		},
		{
			name: "with tiller args",
			releases: []ReleaseSpec{
				{
					Name:            "releaseName",
					Chart:           "foo",
					TillerNamespace: "tillerns",
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--tiller-namespace", "tillerns"}}},
		},
		{
			name: "escaped values",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					SetValues: []SetValue{
						{
							Name:  "someList",
							Value: "a,b,c",
						},
						{
							Name:  "json",
							Value: "{\"name\": \"john\"}",
						},
					},
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--set", "someList=a\\,b\\,c", "--set", "json=\\{\"name\": \"john\"\\}"}}},
		},
		{
			name: "set single value from file",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					SetValues: []SetValue{
						{
							Name:  "foo",
							Value: "FOO",
						},
						{
							Name: "bar",
							File: "path/to/bar",
						},
						{
							Name:  "baz",
							Value: "BAZ",
						},
					},
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--set", "foo=FOO", "--set-file", "bar=path/to/bar", "--set", "baz=BAZ"}}},
		},
		{
			name: "set single array value in an array",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					SetValues: []SetValue{
						{
							Name: "foo.bar[0]",
							Values: []string{
								"A",
								"B",
							},
						},
					},
				},
			},
			helm:         &mockHelmExec{},
			wantReleases: []mockRelease{{"releaseName", []string{"--set", "foo.bar[0]={A,B}"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
			}
			_, errs := state.DiffReleases(tt.helm, []string{}, 1, false, false, false)
			if errs != nil && len(errs) > 0 {
				t.Errorf("unexpected error: %v", errs)
			}
			if !reflect.DeepEqual(tt.helm.diffed, tt.wantReleases) {
				t.Errorf("HelmState.DiffReleases() for [%s] = %v, want %v", tt.name, tt.helm.releases, tt.wantReleases)
			}
		})
	}
}

func TestHelmState_SyncReleasesCleanup(t *testing.T) {
	tests := []struct {
		name                    string
		releases                []ReleaseSpec
		helm                    *mockHelmExec
		expectedNumRemovedFiles int
	}{
		{
			name: "normal release",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
				},
			},
			helm:                    &mockHelmExec{},
			expectedNumRemovedFiles: 0,
		},
		{
			name: "inline values",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					Values: []interface{}{
						map[interface{}]interface{}{
							"someList": "a,b,c",
						},
					},
				},
			},
			helm:                    &mockHelmExec{},
			expectedNumRemovedFiles: 1,
		},
		{
			name: "inline values and values file",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					Values: []interface{}{
						map[interface{}]interface{}{
							"someList": "a,b,c",
						},
						"someFile",
					},
				},
			},
			helm:                    &mockHelmExec{},
			expectedNumRemovedFiles: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numRemovedFiles := 0
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
				readFile: func(f string) ([]byte, error) {
					if f != "someFile" {
						return nil, fmt.Errorf("unexpected file to read: %s", f)
					}
					someFileContent := []byte(`foo: bar
`)
					return someFileContent, nil
				},
				removeFile: func(f string) error {
					numRemovedFiles += 1
					return nil
				},
				fileExists: func(f string) (bool, error) {
					return true, nil
				},
			}
			if errs := state.SyncReleases(tt.helm, []string{}, 1); errs != nil && len(errs) > 0 {
				t.Errorf("unexpected errors: %v", errs)
			}

			if errs := state.Clean(); errs != nil && len(errs) > 0 {
				t.Errorf("unexpected errors: %v", errs)
			}

			if numRemovedFiles != tt.expectedNumRemovedFiles {
				t.Errorf("unexpected number of removed files: expected %d, got %d", tt.expectedNumRemovedFiles, numRemovedFiles)
			}
		})
	}
}

func TestHelmState_DiffReleasesCleanup(t *testing.T) {
	tests := []struct {
		name                    string
		releases                []ReleaseSpec
		helm                    *mockHelmExec
		expectedNumRemovedFiles int
	}{
		{
			name: "normal release",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
				},
			},
			helm:                    &mockHelmExec{},
			expectedNumRemovedFiles: 0,
		},
		{
			name: "inline values",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					Values: []interface{}{
						map[interface{}]interface{}{
							"someList": "a,b,c",
						},
					},
				},
			},
			helm:                    &mockHelmExec{},
			expectedNumRemovedFiles: 1,
		},
		{
			name: "inline values and values file",
			releases: []ReleaseSpec{
				{
					Name:  "releaseName",
					Chart: "foo",
					Values: []interface{}{
						map[interface{}]interface{}{
							"someList": "a,b,c",
						},
						"someFile",
					},
				},
			},
			helm:                    &mockHelmExec{},
			expectedNumRemovedFiles: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numRemovedFiles := 0
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
				readFile: func(f string) ([]byte, error) {
					if f != "someFile" {
						return nil, fmt.Errorf("unexpected file to read: %s", f)
					}
					someFileContent := []byte(`foo: bar
`)
					return someFileContent, nil
				},
				removeFile: func(f string) error {
					numRemovedFiles += 1
					return nil
				},
				fileExists: func(f string) (bool, error) {
					return true, nil
				},
			}
			if _, errs := state.DiffReleases(tt.helm, []string{}, 1, false, false, false); errs != nil && len(errs) > 0 {
				t.Errorf("unexpected errors: %v", errs)
			}

			if errs := state.Clean(); errs != nil && len(errs) > 0 {
				t.Errorf("unexpected errors: %v", errs)
			}

			if numRemovedFiles != tt.expectedNumRemovedFiles {
				t.Errorf("unexpected number of removed files: expected %d, got %d", tt.expectedNumRemovedFiles, numRemovedFiles)
			}
		})
	}
}

func TestHelmState_UpdateDeps(t *testing.T) {
	state := &HelmState{
		basePath: "/src",
		Releases: []ReleaseSpec{
			{
				Chart: "./..",
			},
			{
				Chart: "../examples",
			},
			{
				Chart: "../../helmfile",
			},
			{
				Chart: "published",
			},
			{
				Chart: "published/deeper",
			},
			{
				Chart: ".error",
			},
		},
	}

	want := []string{"/", "/examples", "/helmfile"}
	helm := &mockHelmExec{}
	errs := state.UpdateDeps(helm)
	if !reflect.DeepEqual(helm.charts, want) {
		t.Errorf("HelmState.UpdateDeps() = %v, want %v", helm.charts, want)
	}
	if len(errs) != 0 {
		t.Errorf("HelmState.UpdateDeps() - no errors, but got: %v", len(errs))
	}
}

func TestHelmState_ReleaseStatuses(t *testing.T) {
	tests := []struct {
		name     string
		releases []ReleaseSpec
		helm     *mockHelmExec
		want     []mockRelease
		wantErr  bool
	}{
		{
			name: "happy path",
			releases: []ReleaseSpec{
				{
					Name: "releaseA",
				},
			},
			helm: &mockHelmExec{},
			want: []mockRelease{{"releaseA", []string{}}},
		},
		{
			name: "happy path",
			releases: []ReleaseSpec{
				{
					Name: "error",
				},
			},
			helm:    &mockHelmExec{},
			wantErr: true,
		},
		{
			name: "complain missing values file for desired release",
			releases: []ReleaseSpec{
				{
					Name: "error",
					Values: []interface{}{
						"foo.yaml",
					},
				},
			},
			helm:    &mockHelmExec{},
			wantErr: true,
		},
		{
			name: "should not complain missing values file for undesired release",
			releases: []ReleaseSpec{
				{
					Name: "error",
					Values: []interface{}{
						"foo.yaml",
					},
					Installed: boolValue(false),
				},
			},
			helm:    &mockHelmExec{},
			wantErr: false,
		},
		{
			name: "with tiller args",
			releases: []ReleaseSpec{
				{
					Name:            "releaseA",
					TillerNamespace: "tillerns",
				},
			},
			helm: &mockHelmExec{},
			want: []mockRelease{{"releaseA", []string{"--tiller-namespace", "tillerns"}}},
		},
	}
	for _, tt := range tests {
		i := func(t *testing.T) {
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
				fileExists: func(f string) (bool, error) {
					if f != "foo.yaml" {
						return false, fmt.Errorf("unexpected file: %s", f)
					}
					return true, nil
				},
				readFile: func(f string) ([]byte, error) {
					if f != "foo.yaml" {
						return nil, fmt.Errorf("unexpected file: %s", f)
					}
					return []byte{}, nil
				},
			}
			errs := state.ReleaseStatuses(tt.helm, 1)
			if (errs != nil) != tt.wantErr {
				t.Errorf("ReleaseStatuses() for %s error = %v, wantErr %v", tt.name, errs, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.helm.releases, tt.want) {
				t.Errorf("HelmState.ReleaseStatuses() for [%s] = %v, want %v", tt.name, tt.helm.releases, tt.want)
			}
		}
		t.Run(tt.name, i)
	}
}

func TestHelmState_TestReleasesNoCleanUp(t *testing.T) {
	tests := []struct {
		name            string
		cleanup         bool
		releases        []ReleaseSpec
		helm            *mockHelmExec
		want            []mockRelease
		wantErr         bool
		tillerNamespace string
	}{
		{
			name: "happy path",
			releases: []ReleaseSpec{
				{
					Name: "releaseA",
				},
			},
			helm: &mockHelmExec{},
			want: []mockRelease{{"releaseA", []string{"--timeout", "1"}}},
		},
		{
			name:    "do cleanup",
			cleanup: true,
			releases: []ReleaseSpec{
				{
					Name: "releaseB",
				},
			},
			helm: &mockHelmExec{},
			want: []mockRelease{{"releaseB", []string{"--cleanup", "--timeout", "1"}}},
		},
		{
			name: "happy path",
			releases: []ReleaseSpec{
				{
					Name: "error",
				},
			},
			helm:    &mockHelmExec{},
			wantErr: true,
		},
		{
			name: "with tiller args",
			releases: []ReleaseSpec{
				{
					Name:            "releaseA",
					TillerNamespace: "tillerns",
				},
			},
			helm: &mockHelmExec{},
			want: []mockRelease{{"releaseA", []string{"--timeout", "1", "--tiller-namespace", "tillerns"}}},
		},
	}
	for _, tt := range tests {
		i := func(t *testing.T) {
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
			}
			errs := state.TestReleases(tt.helm, tt.cleanup, 1, 1)
			if (errs != nil) != tt.wantErr {
				t.Errorf("TestReleases() for %s error = %v, wantErr %v", tt.name, errs, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.helm.releases, tt.want) {
				t.Errorf("HelmState.TestReleases() for [%s] = %v, want %v", tt.name, tt.helm.releases, tt.want)
			}
		}
		t.Run(tt.name, i)
	}
}

func TestHelmState_NoReleaseMatched(t *testing.T) {
	releases := []ReleaseSpec{
		{
			Name: "releaseA",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	tests := []struct {
		name    string
		labels  string
		wantErr bool
	}{
		{
			name: "happy path",

			labels:  "foo=bar",
			wantErr: false,
		},
		{
			name:    "name does not exist",
			labels:  "name=releaseB",
			wantErr: false,
		},
		{
			name:    "label does not match anything",
			labels:  "foo=notbar",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		i := func(t *testing.T) {
			state := &HelmState{
				Releases: releases,
				logger:   logger,
			}
			errs := state.FilterReleases([]string{tt.labels})
			if (errs != nil) != tt.wantErr {
				t.Errorf("ReleaseStatuses() for %s error = %v, wantErr %v", tt.name, errs, tt.wantErr)
				return
			}
		}
		t.Run(tt.name, i)
	}
}

func TestHelmState_Delete(t *testing.T) {
	tests := []struct {
		name            string
		deleted         []mockRelease
		wantErr         bool
		desired         *bool
		installed       bool
		purge           bool
		flags           string
		tillerNamespace string
	}{
		{
			name:      "desired and installed (purge=false)",
			wantErr:   false,
			desired:   boolValue(true),
			installed: true,
			purge:     false,
			deleted:   []mockRelease{{"releaseA", []string{}}},
		},
		{
			name:      "desired(default) and installed (purge=false)",
			wantErr:   false,
			desired:   nil,
			installed: true,
			purge:     false,
			deleted:   []mockRelease{{"releaseA", []string{}}},
		},
		{
			name:      "desired and installed (purge=true)",
			wantErr:   false,
			desired:   boolValue(true),
			installed: true,
			purge:     true,
			deleted:   []mockRelease{{"releaseA", []string{"--purge"}}},
		},
		{
			name:      "desired but not installed (purge=false)",
			wantErr:   false,
			desired:   boolValue(true),
			installed: false,
			purge:     false,
			deleted:   []mockRelease{},
		},
		{
			name:      "desired but not installed (purge=true)",
			wantErr:   false,
			desired:   boolValue(true),
			installed: false,
			purge:     true,
			deleted:   []mockRelease{},
		},
		{
			name:      "installed but filtered (purge=false)",
			wantErr:   false,
			desired:   boolValue(false),
			installed: true,
			purge:     false,
			deleted:   []mockRelease{},
		},
		{
			name:      "installed but filtered (purge=true)",
			wantErr:   false,
			desired:   boolValue(false),
			installed: true,
			purge:     true,
			deleted:   []mockRelease{},
		},
		{
			name:      "not installed, and filtered (purge=false)",
			wantErr:   false,
			desired:   boolValue(false),
			installed: false,
			purge:     false,
			deleted:   []mockRelease{},
		},
		{
			name:      "not installed, and filtered (purge=true)",
			wantErr:   false,
			desired:   boolValue(false),
			installed: false,
			purge:     true,
			deleted:   []mockRelease{},
		},
		{
			name:            "with tiller args",
			wantErr:         false,
			desired:         nil,
			installed:       true,
			purge:           true,
			tillerNamespace: "tillerns",
			flags:           "--tiller-namespacetillerns",
			deleted:         []mockRelease{{"releaseA", []string{"--purge", "--tiller-namespace", "tillerns"}}},
		},
	}
	for _, tt := range tests {
		i := func(t *testing.T) {
			release := ReleaseSpec{
				Name:            "releaseA",
				Installed:       tt.desired,
				TillerNamespace: tt.tillerNamespace,
			}
			releases := []ReleaseSpec{
				release,
			}
			state := &HelmState{
				Releases: releases,
				logger:   logger,
			}
			helm := &mockHelmExec{
				lists:   map[listKey]string{},
				deleted: []mockRelease{},
			}
			if tt.installed {
				helm.lists[listKey{filter: "^releaseA$", flags: tt.flags}] = "releaseA"
			}
			errs := state.DeleteReleases(helm, tt.purge)
			if (errs != nil) != tt.wantErr {
				t.Errorf("DeleteReleases() for %s error = %v, wantErr %v", tt.name, errs, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.deleted, helm.deleted) {
				t.Errorf("unexpected deletions happened: expected %v, got %v", tt.deleted, helm.deleted)
			}
		}
		t.Run(tt.name, i)
	}
}
