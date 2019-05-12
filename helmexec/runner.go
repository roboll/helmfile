package helmexec

import (
	"bytes"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	tmpPrefix = "helmfile-"
	tmpSuffix = "-exec"
)

// Runner interface for shell commands
type Runner interface {
	Execute(cmd string, args []string, env map[string]string) ([]byte, error)
}

// ShellRunner implemention for shell commands
type ShellRunner struct {
	Dir string

	logger *zap.SugaredLogger
}

// Execute a shell command
func (shell ShellRunner) Execute(cmd string, args []string, env map[string]string) ([]byte, error) {
	preparedCmd := exec.Command(cmd, args...)
	preparedCmd.Dir = shell.Dir
	preparedCmd.Env = mergeEnv(os.Environ(), env)
	return combinedOutput(preparedCmd, shell.logger)
}

func combinedOutput(c *exec.Cmd, logger *zap.SugaredLogger) ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()

	o := stdout.Bytes()
	e := stderr.Bytes()

	if err != nil {
		// TrimSpace is necessary, because otherwise helmfile prints the redundant new-lines after each error like:
		//
		//   err: release "envoy2" in "helmfile.yaml" failed: exit status 1: Error: could not find a ready tiller pod
		//   <redundant new line!>
		//   err: release "envoy" in "helmfile.yaml" failed: exit status 1: Error: could not find a ready tiller pod
		switch ee := err.(type) {
		case *exec.ExitError:
			// Propagate any non-zero exit status from the external command, rather than throwing it away,
			// so that helmfile could return its own exit code accordingly
			waitStatus := ee.Sys().(syscall.WaitStatus)
			exitStatus := waitStatus.ExitStatus()
			err = ExitError{
				msg:        fmt.Sprintf("%s exited with status %d:\n%s", filepath.Base(c.Path), exitStatus, indent(strings.TrimSpace(string(e)))),
				exitStatus: exitStatus,
			}
		default:
			panic(fmt.Sprintf("unexpected error: %v", err))
		}
	}

	return o, err
}

func indent(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// ExitError is created whenever your shell command exits with a non-zero exit status
type ExitError struct {
	msg    string
	exitStatus int
}

func (e ExitError) Error() string {
	return e.msg
}

func (e ExitError) ExitStatus() int {
	return e.exitStatus
}

func mergeEnv(orig []string, new map[string]string) []string {
	wanted := env2map(orig)
	for k, v := range new {
		wanted[k] = v
	}
	return map2env(wanted)
}

func map2env(wanted map[string]string) []string {
	result := []string{}
	for k, v := range wanted {
		result = append(result, k+"="+v)
	}
	return result
}

func env2map(env []string) map[string]string {
	wanted := map[string]string{}
	for _, cur := range env {
		pair := strings.SplitN(cur, "=", 2)
		wanted[pair[0]] = pair[1]
	}
	return wanted
}
