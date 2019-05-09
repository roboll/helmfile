package helmexec

import (
	"bytes"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"strings"
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

	if len(e) > 0 {
		logger.Debugf("%s\n", e)
	}

	if err != nil {
		// TrimSpace is necessary, because otherwise helmfile prints the redundant new-lines after each error like:
		//
		//   err: release "envoy2" in "helmfile.yaml" failed: exit status 1: Error: could not find a ready tiller pod
		//   <redundant new line!>
		//   err: release "envoy" in "helmfile.yaml" failed: exit status 1: Error: could not find a ready tiller pod
		err = fmt.Errorf("%v: %s", err, strings.TrimSpace(string(e)))
	}

	return o, err
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
