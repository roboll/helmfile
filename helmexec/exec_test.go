package helmexec

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

// Mocking the command-line runner

type mockRunner struct {
	output []byte
	err    error
}

func (mock *mockRunner) Execute(cmd string, args []string) ([]byte, error) {
	return []byte{}, nil
}

func MockExecer(writer io.Writer, kubeContext string) *execer {
	execer := New(writer, kubeContext)
	execer.runner = &mockRunner{}
	return execer
}

// Test methods

func TestNewHelmExec(t *testing.T) {
	buffer := bytes.NewBufferString("something")
	helm := New(buffer, "dev")
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
	helm := New(new(bytes.Buffer), "dev")
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

func Test_AddRepo(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "dev")
	helm.AddRepo("myRepo", "https://repo.example.com/", "cert.pem", "key.pem")
	expected := "exec: helm repo add myRepo https://repo.example.com/ --cert-file cert.pem --key-file key.pem --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.AddRepo("myRepo", "https://repo.example.com/", "", "")
	expected = "exec: helm repo add myRepo https://repo.example.com/ --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.AddRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_UpdateRepo(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "dev")
	helm.UpdateRepo()
	expected := "exec: helm repo update --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.UpdateRepo()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_SyncRelease(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "dev")
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
	helm := MockExecer(&buffer, "dev")
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
	helm := MockExecer(&buffer, "dev")
	helm.DecryptSecret("secretName")
	expected := "exec: helm secrets dec secretName --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DecryptSecret()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DiffRelease(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "dev")
	helm.DiffRelease("release", "chart", "--timeout 10", "--wait")
	expected := "exec: helm diff release chart --timeout 10 --wait --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	buffer.Reset()
	helm.DiffRelease("release", "chart")
	expected = "exec: helm diff release chart --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DiffRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_DeleteRelease(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "dev")
	helm.DeleteRelease("release")
	expected := "exec: helm delete --purge release --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.DeleteRelease()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_ReleaseStatus(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "dev")
	helm.ReleaseStatus("myRelease")
	expected := "exec: helm status myRelease --kube-context dev\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.ReleaseStatus()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}
}

func Test_exec(t *testing.T) {
	var buffer bytes.Buffer
	helm := MockExecer(&buffer, "")
	helm.exec("version")
	expected := "exec: helm version\n"
	if buffer.String() != expected {
		t.Errorf("helmexec.exec()\nactual = %v\nexpect = %v", buffer.String(), expected)
	}

	helm = MockExecer(nil, "dev")
	ret, _ := helm.exec("diff")
	if len(ret) != 0 {
		t.Error("helmexec.exec() - expected empty return value")
	}

	buffer.Reset()
	helm = MockExecer(&buffer, "dev")
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
}
