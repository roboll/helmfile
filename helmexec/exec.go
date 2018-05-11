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
	runner      Runner
	writer      io.Writer
	kubeContext string
	extra       []string
}

// New for running helm commands
func New(writer io.Writer, kubeContext string) *execer {
	return &execer{
		writer:      writer,
		kubeContext: kubeContext,
		runner:      &ShellRunner{},
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
	helm.write(out)
	return err
}

func (helm *execer) UpdateRepo() error {
	out, err := helm.exec("repo", "update")
	helm.write(out)
	return err
}

func (helm *execer) UpdateDeps(chart string) error {
	out, err := helm.exec("dependency", "update", chart)
	helm.write(out)
	return err
}

func (helm *execer) SyncRelease(name, chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"upgrade", "--install", "--reset-values", name, chart}, flags...)...)
	helm.write(out)
	return err
}

func (helm *execer) ReleaseStatus(name string) error {
	out, err := helm.exec(append([]string{"status", name})...)
	if helm.writer != nil {
		helm.writer.Write(out)
	}
	return err
}

func (helm *execer) DecryptSecret(name string) (string, error) {
	out, err := helm.exec(append([]string{"secrets", "dec", name})...)
	helm.write(out)
	return name + ".dec", err
}

func (helm *execer) DiffRelease(name, chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"diff", name, chart}, flags...)...)
	helm.write(out)
	return err
}

func (helm *execer) DeleteRelease(name string, flags ...string) error {
	out, err := helm.exec(append([]string{"delete", name}, flags...)...)
	helm.write(out)
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
	helm.write([]byte(fmt.Sprintf("exec: helm %s\n", strings.Join(cmdargs, " "))))
	return helm.runner.Execute(command, cmdargs)
}

func (helm *execer) write(out []byte) {
	if helm.writer != nil {
		helm.writer.Write(out)
	}
}
