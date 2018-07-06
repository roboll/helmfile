package helmexec

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"go.uber.org/zap"
)

// Mocking the command-line runner

type mockRunner struct {
	output []byte
	err    error
}

func (mock *mockRunner) Execute(cmd string, args []string) ([]byte, error) {
	return []byte{}, nil
}

func MockExecer(logger *zap.SugaredLogger, kubeContext string) *execer {
	execer := New(logger, kubeContext)
	execer.runner = &mockRunner{}
	return execer
}

// Test methods

func TestNewHelmExec(t *testing.T) {
	buffer := bytes.NewBufferString("something")
	logger := NewLogger(buffer, "info")
	helm := New(logger, "dev")
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
	helm := New(NewLogger(os.Stdout, "info"), "dev")
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
	helm := New(NewLogger(os.Stdout, "info"), "dev")
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
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.AddRepo("myRepo", "https://repo.example.com/", "cert.pem", "key.pem", "", "")
	expected := "exec: helm repo add myRepo https://repo.example.com/ --cert-file cert.pem --key-file key.pem --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("myRepo", "https://repo.example.com/", "", "", "", "")
	expected = "exec: helm repo add myRepo https://repo.example.com/ --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("myRepo", "https://repo.example.com/", "", "", "example_user", "example_password")
	expected = "exec: helm repo add myRepo https://repo.example.com/ --username example_user --password example_password --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_UpdateRepo(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.UpdateRepo()
	expected := "exec: helm repo update --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.UpdateRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_SyncRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.SyncRelease("release", "chart", "--timeout 10", "--wait")
	expected := "exec: helm upgrade --install --reset-values release chart --timeout 10 --wait --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.SyncRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SyncRelease("release", "chart")
	expected = "exec: helm upgrade --install --reset-values release chart --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.SyncRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_UpdateDeps(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.UpdateDeps("./chart/foo")
	expected := "exec: helm dependency update ./chart/foo --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.SyncRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SetExtraArgs("--verify")
	helm.UpdateDeps("./chart/foo")
	expected = "exec: helm dependency update ./chart/foo --verify --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DecryptSecret(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.DecryptSecret("secretName")
	expected := "exec: helm secrets dec secretName --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DecryptSecret()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DiffRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.DiffRelease("release", "chart", "--timeout 10", "--wait")
	expected := "exec: helm diff upgrade --allow-unreleased release chart --timeout 10 --wait --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.DiffRelease("release", "chart")
	expected = "exec: helm diff upgrade --allow-unreleased release chart --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DeleteRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.DeleteRelease("release")
	expected := "exec: helm delete release --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DeleteRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}
func Test_DeleteRelease_Flags(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.DeleteRelease("release", "--purge")
	expected := "exec: helm delete release --purge --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DeleteRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_TestRelease(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.TestRelease("release")
	expected := "exec: helm test release --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.TestRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}
func Test_TestRelease_Flags(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.TestRelease("release", "--cleanup", "--timeout", "60")
	expected := "exec: helm test release --cleanup --timeout 60 --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.TestRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_ReleaseStatus(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.ReleaseStatus("myRelease")
	expected := "exec: helm status myRelease --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.ReleaseStatus()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_exec(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "")
	helm.exec("version")
	expected := "exec: helm version\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	helm = MockExecer(logger, "dev")
	ret, _ := helm.exec("diff")
	if len(ret) != 0 {
		t.Error("helmexec.exec() - expected empty return value")
	}

	buffer.Reset()
	helm = MockExecer(logger, "dev")
	helm.exec("diff", "release", "chart", "--timeout 10", "--wait")
	expected = "exec: helm diff release chart --timeout 10 --wait --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.exec("version")
	expected = "exec: helm version --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.SetExtraArgs("foo")
	helm.exec("version")
	expected = "exec: helm version foo --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm = MockExecer(logger, "")
	helm.SetHelmBinary("overwritten")
	helm.exec("version")
	expected = "exec: overwritten version\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_Lint(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.Lint("path/to/chart", "--values", "file.yml")
	expected := "exec: helm lint path/to/chart --values file.yml --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.Lint()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_Fetch(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(&buffer, "info")
	helm := MockExecer(logger, "dev")
	helm.Fetch("chart", "--version", "1.2.3", "--untar", "--untardir", "/tmp/dir")
	expected := "exec: helm fetch chart --version 1.2.3 --untar --untardir /tmp/dir --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.Lint()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}
