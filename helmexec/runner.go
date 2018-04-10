package helmexec

import (
	"io/ioutil"
	"os"
	"os/exec"
)

const (
	tmpPrefix = "helmfile-"
	tmpSuffix = "-exec"
)

// Runner interface for shell commands
type Runner interface {
	Execute(cmd string, args []string) ([]byte, error)
}

// ShellRunner implemention for shell commands
type ShellRunner struct{}

// Execute a shell command
func (shell ShellRunner) Execute(cmd string, args []string) ([]byte, error) {
	dir, err := ioutil.TempDir("", tmpPrefix+cmd+tmpSuffix)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	preparedCmd := exec.Command(cmd, args...)
	preparedCmd.Dir = dir
	return preparedCmd.CombinedOutput()
}
