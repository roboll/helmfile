package helmexec

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

const (
	command = "helm"
)

type execer struct {
	writer      io.Writer
	kubeContext string
	extra       []string
}

func NewHelmExec(writer io.Writer, kubeContext string) Interface {
	return &execer{
		writer:      writer,
		kubeContext: kubeContext,
	}
}

func (helm *execer) SetExtraArgs(args ...string) {
	helm.extra = args
}

func (helm *execer) AddRepo(name, repository, certfile, keyfile string) error {
	var args []string
	args = append(args, "repo", "add", name, repository)
	if certfile != "" && keyfile != "" {
		args = append(args, "--cert-file", certfile, "--key-file", keyfile)
	}
	out, err := helm.exec(args...)
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

func (helm *execer) SyncRelease(name, chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"upgrade", "--install", name, chart}, flags...)...)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func (helm *execer) DiffRelease(name, chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"diff", name, chart}, flags...)...)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func (helm *execer) DeleteRelease(name string) error {
	out, err := helm.exec("delete", "--purge", name)
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
	if helm.kubeContext != "" {
		cmdargs = append(cmdargs, "--kube-context", helm.kubeContext)
	}
	if helm.writer != nil {
		helm.writer.Write([]byte(fmt.Sprintf("exec: helm %s\n", strings.Join(cmdargs, " "))))
	}

	cmd := exec.Command(command, cmdargs...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
