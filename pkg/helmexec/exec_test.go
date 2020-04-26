package helmexec

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"

	"go.uber.org/zap"
)

// Mocking the command-line runner

type mockRunner struct {
	output []byte
	err    error
}

func (mock *mockRunner) Execute(cmd string, args []string, env map[string]string) ([]byte, error) {
	return mock.output, mock.err
}

func MockExecer(logger *zap.SugaredLogger, kubeContext string) *execer {
	execer := New("helm", logger, kubeContext, &mockRunner{})
	return execer
}

// Test methods

func TestNewHelmExec(t *testing.T) {
	buffer := bytes.NewBufferString("something")
	helm := MockExecer(NewLogger(buffer, "debug"), "dev")
	if helm.kubeContext != "dev" {
		t.Error("helmexec.New() - kubeContext")
	}
	if buffer.String() != "something" {
		t.Error("helmexec.New() - changed buffer")
	}
	if len(helm.extra) != 0 {
		t.Error("helmexec.New() - extra args not empty")
	}
}

func Test_SetExtraArgs(t *testing.T) {
	helm := MockExecer(NewLogger(os.Stdout, "info"), "dev")
	helm.SetExtraArgs()
	if len(helm.extra) != 0 {
		t.Error("helmexec.SetExtraArgs() - passing no arguments should not change extra field")
	}
	helm.SetExtraArgs("foo")
	if !reflect.DeepEqual(helm.extra, []string{"foo"}) {
		t.Error("helmexec.SetExtraArgs() - one extra argument missing")
	}
	helm.SetExtraArgs("alpha", "beta")
	if !reflect.DeepEqual(helm.extra, []string{"alpha", "beta"}) {
		t.Error("helmexec.SetExtraArgs() - two extra arguments missing (overwriting the previous value)")
	}
}

func Test_SetHelmBinary(t *testing.T) {
	helm := MockExecer(NewLogger(os.Stdout, "info"), "dev")
	if helm.helmBinary != "helm" {
		t.Error("helmexec.command - default command is not helm")
	}
	helm.SetHelmBinary("foo")
	if helm.helmBinary != "foo" {
		t.Errorf("helmexec.SetHelmBinary() - actual = %s expect = foo", helm.helmBinary)
	}
}

func Test_AddRepo(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.AddRepo("myRepo", "https://repo.example.com/", "", "cert.pem", "key.pem", "", "")
	expected := `Adding repo myRepo https://repo.example.com/
exec: helm repo add myRepo https://repo.example.com/ --cert-file cert.pem --key-file key.pem --kube-context dev
exec: helm repo add myRepo https://repo.example.com/ --cert-file cert.pem --key-file key.pem --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("myRepo", "https://repo.example.com/", "ca.crt", "", "", "", "")
	expected = `Adding repo myRepo https://repo.example.com/
exec: helm repo add myRepo https://repo.example.com/ --ca-file ca.crt --kube-context dev
exec: helm repo add myRepo https://repo.example.com/ --ca-file ca.crt --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("myRepo", "https://repo.example.com/", "", "", "", "", "")
	expected = `Adding repo myRepo https://repo.example.com/
exec: helm repo add myRepo https://repo.example.com/ --kube-context dev
exec: helm repo add myRepo https://repo.example.com/ --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("myRepo", "https://repo.example.com/", "", "", "", "example_user", "example_password")
	expected = `Adding repo myRepo https://repo.example.com/
exec: helm repo add myRepo https://repo.example.com/ --username example_user --password example_password --kube-context dev
exec: helm repo add myRepo https://repo.example.com/ --username example_user --password example_password --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("", "https://repo.example.com/", "", "", "", "", "")
	expected = `empty field name

`
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_UpdateRepo(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.UpdateRepo()
	expected := `Updating repo
exec: helm repo update --kube-context dev
exec: helm repo update --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.UpdateRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_SyncRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.SyncRelease(HelmContext{}, "release", "chart", "--timeout 10", "--wait")
	expected := `Upgrading release=release, chart=chart
exec: helm upgrade --install --reset-values release chart --timeout 10 --wait --kube-context dev
exec: helm upgrade --install --reset-values release chart --timeout 10 --wait --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.SyncRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SyncRelease(HelmContext{}, "release", "chart")
	expected = `Upgrading release=release, chart=chart
exec: helm upgrade --install --reset-values release chart --kube-context dev
exec: helm upgrade --install --reset-values release chart --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.SyncRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_SyncReleaseTillerless(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.SyncRelease(HelmContext{Tillerless: true, TillerNamespace: "foo"}, "release", "chart",
		"--timeout 10", "--wait")
	expected := `Upgrading release=release, chart=chart
exec: helm tiller run foo -- helm upgrade --install --reset-values release chart --timeout 10 --wait --kube-context dev
exec: helm tiller run foo -- helm upgrade --install --reset-values release chart --timeout 10 --wait --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.SyncRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_UpdateDeps(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.UpdateDeps("./chart/foo")
	expected := `Updating dependency ./chart/foo
exec: helm dependency update ./chart/foo --kube-context dev
exec: helm dependency update ./chart/foo --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.UpdateDeps()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SetExtraArgs("--verify")
	helm.UpdateDeps("./chart/foo")
	expected = `Updating dependency ./chart/foo
exec: helm dependency update ./chart/foo --verify --kube-context dev
exec: helm dependency update ./chart/foo --verify --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_BuildDeps(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.BuildDeps("foo", "./chart/foo")
	expected := `Building dependency release=foo, chart=./chart/foo
exec: helm dependency build ./chart/foo --kube-context dev
exec: helm dependency build ./chart/foo --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.BuildDeps()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SetExtraArgs("--verify")
	helm.BuildDeps("foo", "./chart/foo")
	expected = `Building dependency release=foo, chart=./chart/foo
exec: helm dependency build ./chart/foo --verify --kube-context dev
exec: helm dependency build ./chart/foo --verify --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.BuildDeps()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DecryptSecret(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.DecryptSecret(HelmContext{}, "secretName")
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	// Run again for caching
	helm.DecryptSecret(HelmContext{}, "secretName")

	expected := fmt.Sprintf(`Preparing to decrypt secret %v/secretName
Decrypting secret %s/secretName
exec: helm secrets dec %s/secretName --kube-context dev
exec: helm secrets dec %s/secretName --kube-context dev: 
Preparing to decrypt secret %s/secretName
Found secret in cache %s/secretName
`, cwd, cwd, cwd, cwd, cwd, cwd)
	if buffer.String() != expected {
		t.Errorf("helmexec.DecryptSecret()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DiffRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.DiffRelease(HelmContext{}, "release", "chart", false, "--timeout 10", "--wait")
	expected := `Comparing release=release, chart=chart
exec: helm diff upgrade --reset-values --allow-unreleased release chart --timeout 10 --wait --kube-context dev
exec: helm diff upgrade --reset-values --allow-unreleased release chart --timeout 10 --wait --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.DiffRelease(HelmContext{}, "release", "chart", false)
	expected = `Comparing release=release, chart=chart
exec: helm diff upgrade --reset-values --allow-unreleased release chart --kube-context dev
exec: helm diff upgrade --reset-values --allow-unreleased release chart --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DiffReleaseTillerless(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.DiffRelease(HelmContext{Tillerless: true}, "release", "chart", false, "--timeout 10", "--wait")
	expected := `Comparing release=release, chart=chart
exec: helm tiller run -- helm diff upgrade --reset-values --allow-unreleased release chart --timeout 10 --wait --kube-context dev
exec: helm tiller run -- helm diff upgrade --reset-values --allow-unreleased release chart --timeout 10 --wait --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DeleteRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.DeleteRelease(HelmContext{}, "release")
	expected := `Deleting release
exec: helm delete release --kube-context dev
exec: helm delete release --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.DeleteRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}
func Test_DeleteRelease_Flags(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.DeleteRelease(HelmContext{}, "release", "--purge")
	expected := `Deleting release
exec: helm delete release --purge --kube-context dev
exec: helm delete release --purge --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.DeleteRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_TestRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.TestRelease(HelmContext{}, "release")
	expected := `Testing release
exec: helm test release --kube-context dev
exec: helm test release --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.TestRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}
func Test_TestRelease_Flags(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.TestRelease(HelmContext{}, "release", "--cleanup", "--timeout", "60")
	expected := `Testing release
exec: helm test release --cleanup --timeout 60 --kube-context dev
exec: helm test release --cleanup --timeout 60 --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.TestRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_ReleaseStatus(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.ReleaseStatus(HelmContext{}, "myRelease")
	expected := `Getting status myRelease
exec: helm status myRelease --kube-context dev
exec: helm status myRelease --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.ReleaseStatus()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_exec(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "")
	env := map[string]string{}
	helm.exec([]string{"version"}, env)
	expected := `exec: helm version
exec: helm version: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	helm = MockExecer(logger, "dev")
	ret, _ := helm.exec([]string{"diff"}, env)
	if len(ret) != 0 {
		t.Error("helmexec.exec() - expected empty return value")
	}

	buffer.Reset()
	helm = MockExecer(logger, "dev")
	helm.exec([]string{"diff", "release", "chart", "--timeout 10", "--wait"}, env)
	expected = `exec: helm diff release chart --timeout 10 --wait --kube-context dev
exec: helm diff release chart --timeout 10 --wait --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.exec([]string{"version"}, env)
	expected = `exec: helm version --kube-context dev
exec: helm version --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SetExtraArgs("foo")
	helm.exec([]string{"version"}, env)
	expected = `exec: helm version foo --kube-context dev
exec: helm version foo --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm = MockExecer(logger, "")
	helm.SetHelmBinary("overwritten")
	helm.exec([]string{"version"}, env)
	expected = `exec: overwritten version
exec: overwritten version: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_Lint(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.Lint("release", "path/to/chart", "--values", "file.yml")
	expected := `Linting release=release, chart=path/to/chart
exec: helm lint path/to/chart --values file.yml --kube-context dev
exec: helm lint path/to/chart --values file.yml --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.Lint()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_Fetch(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.Fetch("chart", "--version", "1.2.3", "--untar", "--untardir", "/tmp/dir")
	expected := `Fetching chart
exec: helm fetch chart --version 1.2.3 --untar --untardir /tmp/dir --kube-context dev
exec: helm fetch chart --version 1.2.3 --untar --untardir /tmp/dir --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.Lint()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

var logLevelTests = map[string]string{
	"debug": `Adding repo myRepo https://repo.example.com/
exec: helm repo add myRepo https://repo.example.com/ --username example_user --password example_password
exec: helm repo add myRepo https://repo.example.com/ --username example_user --password example_password: 
`,
	"info": `Adding repo myRepo https://repo.example.com/
`,
	"warn": ``,
}

func Test_LogLevels(t *testing.T) {
	var buffer bytes.Buffer
	for logLevel, expected := range logLevelTests {
		buffer.Reset()
		logger := NewLogger(&buffer, logLevel)
		helm := MockExecer(logger, "")
		helm.AddRepo("myRepo", "https://repo.example.com/", "", "", "", "example_user", "example_password")
		if buffer.String() != expected {
			t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
		}
	}
}

func Test_getTillerlessEnv(t *testing.T) {
	context := HelmContext{Tillerless: true, TillerNamespace: "foo", WorkerIndex: 1}

	os.Unsetenv("KUBECONFIG")
	actual := context.getTillerlessEnv()
	if val, found := actual["HELM_TILLER_SILENT"]; !found || val != "true" {
		t.Errorf("getTillerlessEnv() HELM_TILLER_SILENT\nactual = %s\nexpect = true", val)
	}
	// This feature is disabled until it is fixed in helm
	/*if val, found := actual["HELM_TILLER_PORT"]; !found || val != "44135" {
		t.Errorf("getTillerlessEnv() HELM_TILLER_PORT\nactual = %s\nexpect = 44135", val)
	}*/
	if val, found := actual["KUBECONFIG"]; found {
		t.Errorf("getTillerlessEnv() KUBECONFIG\nactual = %s\nexpect = nil", val)
	}

	os.Setenv("KUBECONFIG", "toto")
	actual = context.getTillerlessEnv()
	cwd, _ := os.Getwd()
	expected := path.Join(cwd, "toto")
	if val, found := actual["KUBECONFIG"]; !found || val != expected {
		t.Errorf("getTillerlessEnv() KUBECONFIG\nactual = %s\nexpect = %s", val, expected)
	}
	os.Unsetenv("KUBECONFIG")
}

func Test_mergeEnv(t *testing.T) {
	actual := env2map(mergeEnv([]string{"A=1", "B=c=d", "E=2"}, map[string]string{"B": "3", "F": "4"}))
	expected := map[string]string{"A": "1", "B": "3", "E": "2", "F": "4"}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("mergeEnv()\nactual = %v\nexpect = %v", actual, expected)
	}
}

func Test_Template(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "debug")
	helm := MockExecer(logger, "dev")
	helm.TemplateRelease("release", "path/to/chart", "--values", "file.yml")
	expected := `Templating release=release, chart=path/to/chart
exec: helm template path/to/chart --name release --values file.yml --kube-context dev
exec: helm template path/to/chart --name release --values file.yml --kube-context dev: 
`
	if buffer.String() != expected {
		t.Errorf("helmexec.Template()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_IsHelm3(t *testing.T) {
	helm2Runner := mockRunner{output: []byte("Client: v2.16.0+ge13bc94\n")}
	helm := New("helm", NewLogger(os.Stdout, "info"), "dev", &helm2Runner)
	if helm.IsHelm3() {
		t.Error("helmexec.IsHelm3() - Detected Helm 3 with Helm 2 version")
	}

	helm3Runner := mockRunner{output: []byte("v3.0.0+ge29ce2a\n")}
	helm = New("helm", NewLogger(os.Stdout, "info"), "dev", &helm3Runner)
	if !helm.IsHelm3() {
		t.Error("helmexec.IsHelm3() - Failed to detect Helm 3")
	}

	os.Setenv("HELMFILE_HELM3", "1")
	helm2Runner = mockRunner{output: []byte("Client: v2.16.0+ge13bc94\n")}
	helm = New("helm", NewLogger(os.Stdout, "info"), "dev", &helm2Runner)
	if !helm.IsHelm3() {
		t.Error("helmexec.IsHelm3() - Helm3 not detected when HELMFILE_HELM3 is set")
	}
	os.Setenv("HELMFILE_HELM3", "")
}

func Test_GetVersion(t *testing.T) {
	helm2Runner := mockRunner{output: []byte("Client: v2.16.1+ge13bc94\n")}
	helm := New("helm", NewLogger(os.Stdout, "info"), "dev", &helm2Runner)
	ver := helm.GetVersion()
	if ver.Major != 2 || ver.Minor != 16 || ver.Patch != 1 {
		t.Error(fmt.Sprintf("helmexec.GetVersion - did not detect correct Helm2 version; it was: %+v", ver))
	}

	helm3Runner := mockRunner{output: []byte("v3.2.4+ge29ce2a\n")}
	helm = New("helm", NewLogger(os.Stdout, "info"), "dev", &helm3Runner)
	ver = helm.GetVersion()
	if ver.Major != 3 || ver.Minor != 2 || ver.Patch != 4 {
		t.Error(fmt.Sprintf("helmexec.GetVersion - did not detect correct Helm3 version; it was: %+v", ver))
	}
}

func Test_IsVersionAtLeast(t *testing.T) {
	helm2Runner := mockRunner{output: []byte("Client: v2.16.1+ge13bc94\n")}
	helm := New("helm", NewLogger(os.Stdout, "info"), "dev", &helm2Runner)
	if !helm.IsVersionAtLeast(2, 1) {
		t.Error("helmexec.IsVersionAtLeast - 2.16.1 not atleast 2.1")
	}

	if helm.IsVersionAtLeast(2, 19) {
		t.Error("helmexec.IsVersionAtLeast - 2.16.1 is atleast 2.19")
	}

	if helm.IsVersionAtLeast(3, 2) {
		t.Error("helmexec.IsVersionAtLeast - 2.16.1 is atleast 3.2")
	}

}
