package helmexec

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"go.uber.org/zap"
)

const (
	tmpPrefix = "helmfile-"
	tmpSuffix = "-exec"
)

// Runner interface for shell commands
type Runner interface {
	Execute(cmd string, args []string, env map[string]string) ([]byte, error)
	OutputInLine(c *exec.Cmd) ([]byte, error)
}

// ShellRunner implemention for shell commands
type ShellRunner struct {
	Dir string

	Logger *zap.SugaredLogger
}

// Execute a shell command
func (shell ShellRunner) Execute(cmd string, args []string, env map[string]string) ([]byte, error) {
	preparedCmd := exec.Command(cmd, args...)
	preparedCmd.Dir = shell.Dir
	preparedCmd.Env = mergeEnv(os.Environ(), env)
	return shell.OutputInLine(preparedCmd)
}

func (shell ShellRunner) OutputInLine(c *exec.Cmd) ([]byte, error) {
	var errorStream string
	var outputStream string
	var combinedStream string

	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, errors.New("exec: Stderr already set")
	}

	err = c.Start()
	if err != nil {
		panic(fmt.Sprintf("unexpected error: %v", err))
	}

	combined := make(chan string, 2)

	var wg sync.WaitGroup

	scannerStdout := bufio.NewScanner(stdout)
	wg.Add(1)
	go func() {
		for scannerStdout.Scan() {
			text := scannerStdout.Text()
			if strings.TrimSpace(text) != "" {
				outputStream += text + "\n"
				combined <- text
			}
		}
		wg.Done()
	}()

	scannerStderr := bufio.NewScanner(stderr)
	wg.Add(1)
	go func() {
		for scannerStderr.Scan() {
			text := scannerStderr.Text()
			if strings.TrimSpace(text) != "" {
				errorStream += text + "\n"
				combined <- text
			}
		}
		wg.Done()
	}()
	go func() {
		wg.Wait()
		close(combined)
	}()

	for t := range combined {
		combinedStream += t + "\n"
		shell.Logger.Debug(t)
	}

	err = c.Wait()
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
			// err = newExitError(c.Path, c.Args, exitStatus, ee, string(stderr), string(stdout))
			shell.Logger.Debug("DASDSADASDASDASDASDADASDSASDASDASDASDAS")
			err = newExitError(c.Path, c.Args, exitStatus, ee, errorStream, combinedStream)
		default:
			panic(fmt.Sprintf("unexpected error: %v", err))
		}
	}

	return []byte(outputStream), err
}

func Output(c *exec.Cmd) ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var combined bytes.Buffer
	c.Stdout = io.MultiWriter(&stdout, &combined)
	c.Stderr = io.MultiWriter(&stderr, &combined)
	err := c.Run()

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
			err = newExitError(c.Path, c.Args, exitStatus, ee, stderr.String(), combined.String())
		default:
			panic(fmt.Sprintf("unexpected error: %v", err))
		}
	}

	return stdout.Bytes(), err
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
