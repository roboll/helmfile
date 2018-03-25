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

type Runner interface {
	Execute(cmd string, args []string) ([]byte, error)
}

type CliRunner struct {}

func (cli CliRunner) Execute(cmd string, args []string) ([]byte, error) {
	dir, err := ioutil.TempDir("", tmpPrefix + cmd + tmpSuffix)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	preparedCmd := exec.Command(cmd, args...)
	preparedCmd.Dir = dir
	return preparedCmd.CombinedOutput()
}
