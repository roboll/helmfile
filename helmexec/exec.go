package helmexec

import (
	"fmt"
	"io"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	command = "helm"
)

type execer struct {
	helmBinary  string
	runner      Runner
	writer      io.Writer
	kubeContext string
	logger      *zap.SugaredLogger
	extra       []string
}

// New for running helm commands
func New(writer io.Writer, kubeContext string) *execer {
	var cfg zapcore.EncoderConfig
	cfg.MessageKey = "message"
	var out zapcore.WriteSyncer
	if writer != nil {
		out = zapcore.AddSync(writer)
	} else {
		out = zapcore.AddSync(os.Stdout)
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(cfg),
		out,
		zap.DebugLevel,
	)
	return &execer{
		helmBinary:  command,
		writer:      writer,
		kubeContext: kubeContext,
		logger:      zap.New(core).Sugar(),
		runner:      &ShellRunner{},
	}
}

func (helm *execer) SetExtraArgs(args ...string) {
	helm.extra = args
}

func (helm *execer) SetHelmBinary(bin string) {
	helm.helmBinary = bin
}

func (helm *execer) AddRepo(name, repository, certfile, keyfile, username, password string) error {
	var args []string
	args = append(args, "repo", "add", name, repository)
	if certfile != "" && keyfile != "" {
		args = append(args, "--cert-file", certfile, "--key-file", keyfile)
	}
	if username != "" && password != "" {
		args = append(args, "--username", username, "--password", password)
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
	helm.write(out)
	return err
}

func (helm *execer) DecryptSecret(name string) (string, error) {
	out, err := helm.exec(append([]string{"secrets", "dec", name})...)
	helm.write(out)
	return name + ".dec", err
}

func (helm *execer) DiffRelease(name, chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"diff", "upgrade", "--allow-unreleased", name, chart}, flags...)...)
	helm.write(out)
	return err
}

func (helm *execer) Lint(chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"lint", chart}, flags...)...)
	helm.write(out)
	return err
}

func (helm *execer) Fetch(chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"fetch", chart}, flags...)...)
	helm.write(out)
	return err
}

func (helm *execer) DeleteRelease(name string, flags ...string) error {
	out, err := helm.exec(append([]string{"delete", name}, flags...)...)
	helm.write(out)
	return err
}

func (helm *execer) TestRelease(name string, flags ...string) error {
	out, err := helm.exec(append([]string{"test", name}, flags...)...)
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
	helm.write([]byte(fmt.Sprintf("exec: %s %s", helm.helmBinary, strings.Join(cmdargs, " "))))
	return helm.runner.Execute(helm.helmBinary, cmdargs)
}

func (helm *execer) write(out []byte) {
	if len(out) > 0 {
		helm.logger.Info(string(out))
	}
}
