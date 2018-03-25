package helmexec

import (
	"fmt"
	"io"
	"strings"
)

const (
	command = "helm"
)

type execer struct {
	runner		Runner
	writer      io.Writer
	kubeContext string
	extra       []string
}

func NewHelmExec(writer io.Writer, kubeContext string) Interface {
	return &execer{
		writer:      writer,
		kubeContext: kubeContext,
		runner:		 new(CliRunner),
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

func (helm *execer) DecryptSecret(name string) (string, error) {
	out, err := helm.exec(append([]string{"secrets", "dec", name})...)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return name + ".dec", err
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
	return helm.runner.Execute(command, cmdargs)
}

// for unit testing
func (helm *execer) setRunner(runner Runner) {
	helm.runner = runner
}
func (helm *execer) getExtra() []string {
	return helm.extra
}
func (helm *execer) getKubeContent() string {
	return helm.kubeContext
}
func (helm *execer) getWriter() io.Writer {
	return helm.writer
}