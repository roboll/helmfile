package state

import (
	"io/ioutil"
	"os"
	"path/filepath"
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
			args, err := state.flagsForUpgrade(helm, tt.release, 0)
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
			want: true,
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
		{
			name: "absolute path",
			args: args{
				chart: "/foo/bar/baz",
			},
			want: true,
		},
		{
			name: "local chart in 3-level deep dir",
			args: args{
				chart: "foo/bar/baz",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLocalChart(tt.args.chart); got != tt.want {
				t.Errorf("%s(\"%s\") isLocalChart(): got %v, want %v", tt.name, tt.args.chart, got, tt.want)
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

	updateDepsCallbacks map[string]func(string) error
}

type mockRelease struct {
	name  string
	flags []string
}

type mockAffected struct {
	upgraded []*mockRelease
	deleted  []*mockRelease
	failed   []*mockRelease
}

func (helm *mockHelmExec) UpdateDeps(chart string) error {
	if strings.Contains(chart, "error") {
		return fmt.Errorf("simulated UpdateDeps failure for chart: %s", chart)
	}
	helm.charts = append(helm.charts, chart)

	if helm.updateDepsCallbacks != nil {
		callback, exists := helm.updateDepsCallbacks[chart]
		if exists {
			if err := callback(chart); err != nil {
				return err
			}
		}
	}
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
func (helm *mockHelmExec) ReleaseStatus(context helmexec.HelmContext, release string, flags ...string) error {
	if strings.Contains(release, "error") {
		return errors.New("error")
	}
	helm.releases = append(helm.releases, mockRelease{name: release, flags: flags})
	return nil
}
func (helm *mockHelmExec) DeleteRelease(context helmexec.HelmContext, name string, flags ...string) error {
	if strings.Contains(name, "error") {
		return errors.New("error")
	}
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
			if _ = state.SyncReleases(&AffectedReleases{}, tt.helm, []string{}, 1); !reflect.DeepEqual(tt.helm.releases, tt.wantReleases) {
				t.Errorf("HelmState.SyncReleases() for [%s] = %v, want %v", tt.name, tt.helm.releases, tt.wantReleases)
			}
		})
	}
}

func TestHelmState_SyncReleasesAffectedRealeases(t *testing.T) {
	no := false
	tests := []struct {
		name         string
		releases     []ReleaseSpec
		installed    []bool
		wantAffected mockAffected
	}{
		{
			name: "2 release",
			releases: []ReleaseSpec{
				{
					Name:  "releaseNameFoo",
					Chart: "foo",
				},
				{
					Name:  "releaseNameBar",
					Chart: "bar",
				},
			},
			wantAffected: mockAffected{[]*mockRelease{{"releaseNameFoo", []string{}}, {"releaseNameBar", []string{}}}, nil, nil},
		},
		{
			name: "2 removed",
			releases: []ReleaseSpec{
				{
					Name:      "releaseNameFoo",
					Chart:     "foo",
					Installed: &no,
				},
				{
					Name:      "releaseNameBar",
					Chart:     "foo",
					Installed: &no,
				},
			},
			installed:    []bool{true, true},
			wantAffected: mockAffected{nil, []*mockRelease{{"releaseNameFoo", []string{}}, {"releaseNameBar", []string{}}}, nil},
		},
		{
			name: "2 errors",
			releases: []ReleaseSpec{
				{
					Name:  "releaseNameFoo-error",
					Chart: "foo",
				},
				{
					Name:  "releaseNameBar-error",
					Chart: "foo",
				},
			},
			wantAffected: mockAffected{nil, nil, []*mockRelease{{"releaseNameFoo-error", []string{}}, {"releaseNameBar-error", []string{}}}},
		},
		{
			name: "1 removed, 1 new, 1 error",
			releases: []ReleaseSpec{
				{
					Name:  "releaseNameFoo",
					Chart: "foo",
				},
				{
					Name:      "releaseNameBar",
					Chart:     "foo",
					Installed: &no,
				},
				{
					Name:  "releaseNameFoo-error",
					Chart: "foo",
				},
			},
			installed:    []bool{true, true, true},
			wantAffected: mockAffected{[]*mockRelease{{"releaseNameFoo", []string{}}}, []*mockRelease{{"releaseNameBar", []string{}}}, []*mockRelease{{"releaseNameFoo-error", []string{}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				Releases: tt.releases,
				logger:   logger,
			}
			helm := &mockHelmExec{
				lists: map[listKey]string{},
			}
			//simulate the release is already installed
			for i, release := range tt.releases {
				if tt.installed != nil && tt.installed[i] {
					helm.lists[listKey{filter: "^" + release.Name + "$"}] = release.Name
				}
			}

			affectedReleases := AffectedReleases{}
			if err := state.SyncReleases(&affectedReleases, helm, []string{}, 1); err != nil {
				if !testEq(affectedReleases.Failed, tt.wantAffected.failed) {
					t.Errorf("HelmState.SynchAffectedRelease() error failed for [%s] = %v, want %v", tt.name, affectedReleases.Failed, tt.wantAffected.failed)
				} //else expected error
			}
			if !testEq(affectedReleases.Upgraded, tt.wantAffected.upgraded) {
				t.Errorf("HelmState.SynchAffectedRelease() upgrade failed for [%s] = %v, want %v", tt.name, affectedReleases.Upgraded, tt.wantAffected.upgraded)
			}
			if !testEq(affectedReleases.Deleted, tt.wantAffected.deleted) {
				t.Errorf("HelmState.SynchAffectedRelease() deleted failed for [%s] = %v, want %v", tt.name, affectedReleases.Deleted, tt.wantAffected.deleted)
			}
		})
	}
}

func testEq(a []*ReleaseSpec, b []*mockRelease) bool {

	// If one is nil, the other must also be nil.
	if (a == nil) != (b == nil) {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].Name != b[i].name {
			return false
		}
	}

	return true
}

func TestGetDeployedVersion(t *testing.T) {
	tests := []struct {
		name             string
		release          ReleaseSpec
		listResult       string
		installedVersion string
	}{
		{
			name: "chart version",
			release: ReleaseSpec{
				Name:  "foo",
				Chart: "../../foo-bar",
			},
			listResult: `NAME 	REVISION	UPDATED                 	STATUS  	CHART                      	APP VERSION	NAMESPACE
										foo	1       	Wed Apr 17 17:39:04 2019	DEPLOYED	foo-bar-2.0.4	0.1.0      	default`,
			installedVersion: "2.0.4",
		},
		{
			name: "chart version with a dash",
			release: ReleaseSpec{
				Name:  "foo-bar",
				Chart: "registry/foo-bar",
			},
			listResult: `NAME 	REVISION	UPDATED                 	STATUS  	CHART                      	APP VERSION	NAMESPACE
										foo	1       	Wed Apr 17 17:39:04 2019	DEPLOYED	foo-bar-1.0.0-alpha.1	0.1.0      	default`,
			installedVersion: "1.0.0-alpha.1",
		},
		{
			name: "chart version with dash and plus",
			release: ReleaseSpec{
				Name:  "foo-bar",
				Chart: "registry/foo-bar",
			},
			listResult: `NAME 	REVISION	UPDATED                 	STATUS  	CHART                      	APP VERSION	NAMESPACE
										foo	1       	Wed Apr 17 17:39:04 2019	DEPLOYED	foo-bar-1.0.0-alpha+001	0.1.0      	default`,
			installedVersion: "1.0.0-alpha+001",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &HelmState{
				Releases: []ReleaseSpec{tt.release},
				logger:   logger,
			}
			helm := &mockHelmExec{
				lists: map[listKey]string{},
			}
			//simulate the helm.list call result
			helm.lists[listKey{filter: "^" + tt.release.Name + "$"}] = tt.listResult

			affectedReleases := AffectedReleases{}
			state.SyncReleases(&affectedReleases, helm, []string{}, 1)

			if state.Releases[0].installedVersion != tt.installedVersion {
				t.Errorf("HelmState.TestGetDeployedVersion() failed for [%s] = %v, want %v", tt.name, state.Releases[0].installedVersion, tt.installedVersion)
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
			if errs := state.SyncReleases(&AffectedReleases{}, tt.helm, []string{}, 1); errs != nil && len(errs) > 0 {
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
	helm := &mockHelmExec{
		updateDepsCallbacks: map[string]func(string) error{},
	}

	var generatedDir string
	tempDir := func(dir, prefix string) (string, error) {
		var err error
		generatedDir, err = ioutil.TempDir(dir, prefix)
		if err != nil {
			return "", err
		}
		helm.updateDepsCallbacks[generatedDir] = func(chart string) error {
			content := []byte(`dependencies:
- name: envoy
  repository: https://kubernetes-charts.storage.googleapis.com
  version: 1.5.0
- name: envoy
  repository: https://kubernetes-charts.storage.googleapis.com
  version: 1.4.0
digest: sha256:8194b597c85bb3d1fee8476d4a486e952681d5c65f185ad5809f2118bc4079b5
generated: 2019-05-16T15:42:45.50486+09:00
`)
			filename := filepath.Join(generatedDir, "requirements.lock")
			logger.Debugf("test: writing %s: %s", filename, content)
			return ioutil.WriteFile(filename, content, 0644)
		}
		return generatedDir, nil
	}

	logger := helmexec.NewLogger(os.Stderr, "debug")
	state := &HelmState{
		basePath: "/src",
		FilePath: "/src/helmfile.yaml",
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
				Chart:   "stable/envoy",
				Version: "1.5.0",
			},
			{
				Chart:   "stable/envoy",
				Version: "1.4.0",
			},
		},
		Repositories: []RepositorySpec{
			{
				Name: "stable",
				URL:  "https://kubernetes-charts.storage.googleapis.com",
			},
		},
		tempDir: tempDir,
		logger:  logger,
	}

	errs := state.UpdateDeps(helm)
	want := []string{"/", "/examples", "/helmfile", "/src/published", generatedDir}
	if !reflect.DeepEqual(helm.charts, want) {
		t.Errorf("HelmState.UpdateDeps() = %v, want %v", helm.charts, want)
	}
	if len(errs) != 0 {
		t.Errorf("HelmState.UpdateDeps() - no errors, but got %d: %v", len(errs), errs)
	}

	resolved, err := state.ResolveDeps()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resolved.Releases[5].Version != "1.5.0" {
		t.Errorf("unexpected version number: expected=1.5.0, got=%s", resolved.Releases[5].Version)
	}
	if resolved.Releases[6].Version != "1.4.0" {
		t.Errorf("unexpected version number: expected=1.4.0, got=%s", resolved.Releases[6].Version)
	}
}

func TestHelmState_ResolveDeps_NoLockFile(t *testing.T) {
	logger := helmexec.NewLogger(os.Stderr, "debug")
	state := &HelmState{
		basePath: "/src",
		FilePath: "/src/helmfile.yaml",
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
				Chart: "stable/envoy",
			},
		},
		Repositories: []RepositorySpec{
			{
				Name: "stable",
				URL:  "https://kubernetes-charts.storage.googleapis.com",
			},
		},
		logger: logger,
		readFile: func(f string) ([]byte, error) {
			if f != "helmfile.lock" {
				return nil, fmt.Errorf("stub: unexpected file: %s", f)
			}
			return nil, os.ErrNotExist
		},
	}

	_, err := state.ResolveDeps()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
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
			state.Selectors = []string{tt.labels}
			errs := state.FilterReleases()
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
			name:      "desired(default) and installed (purge=false) but error",
			wantErr:   true,
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
			name := "releaseA"
			if tt.wantErr {
				name = "releaseA-error"
			}
			release := ReleaseSpec{
				Name:            name,
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
				helm.lists[listKey{filter: "^" + name + "$", flags: tt.flags}] = name
			}
			affectedReleases := AffectedReleases{}
			errs := state.DeleteReleases(&affectedReleases, helm, tt.purge)
			if errs != nil {
				if !tt.wantErr || len(affectedReleases.Failed) != 1 || affectedReleases.Failed[0].Name != release.Name {
					t.Errorf("DeleteReleases() for %s error = %v, wantErr %v", tt.name, errs, tt.wantErr)
					return
				}
			} else if !(reflect.DeepEqual(tt.deleted, helm.deleted) && (len(affectedReleases.Deleted) == len(tt.deleted))) {
				t.Errorf("unexpected deletions happened: expected %v, got %v", &affectedReleases.Deleted, tt.deleted)
			}
		}
		t.Run(tt.name, i)
	}
}
