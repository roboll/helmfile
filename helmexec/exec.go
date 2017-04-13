package helmexec

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	command = "helm"
)

type execer struct {
	writer io.Writer
	extra  []string
}

func NewHelmExec(writer io.Writer) Interface {
	return &execer{writer: writer}
}

func (helm *execer) SetExtraArgs(args ...string) {
	helm.extra = args
}

func (helm *execer) AddRepo(name, repository string) error {
	out, err := helm.exec("repo", "add", name, repository)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func (helm *execer) UpdateRepo() error {
	out, err := helm.exec("repo", "update")
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func normalizeChart(chart string) (string, error) {
	regex, err := regexp.Compile("^[.]?./")
	if err != nil {
		return "", err
	}
	if !regex.MatchString(chart) {
		return chart, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path := filepath.Join(wd, chart)
	return path, nil
}

func (helm *execer) SyncChart(name, chart string, flags ...string) error {
	chart, err := normalizeChart(chart)
	if err != nil {
		return err
	}
	out, err := helm.exec(append([]string{"upgrade", "--install", name, chart}, flags...)...)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func (helm *execer) DeleteChart(name string) error {
	out, err := helm.exec("delete", name)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func (helm *execer) exec(args ...string) ([]byte, error) {
	dir, err := ioutil.TempDir("", "helmfile-exec")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	cmdargs := args
	if len(helm.extra) > 0 {
		cmdargs = append(cmdargs, helm.extra...)
	}
	if helm.writer != nil {
		helm.writer.Write([]byte(fmt.Sprintf("exec: helm %s\n", strings.Join(cmdargs, " "))))
	}

	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
