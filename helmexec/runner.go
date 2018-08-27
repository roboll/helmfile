package helmexec

import (
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
	preparedCmd := exec.Command(cmd, args...)
	return preparedCmd.CombinedOutput()
}
