package helmexec

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	command = "helm"
)

type execer struct {
	helmBinary      string
	runner          Runner
	logger          *zap.SugaredLogger
	kubeContext     string
	extra           []string
	decryptionMutex sync.Mutex
}

func NewLogger(writer io.Writer, logLevel string) *zap.SugaredLogger {
	var cfg zapcore.EncoderConfig
	cfg.MessageKey = "message"
	out := zapcore.AddSync(writer)
	var level zapcore.Level
	err := level.Set(logLevel)
	if err != nil {
		panic(err)
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(cfg),
		out,
		level,
	)
	return zap.New(core).Sugar()
}

// New for running helm commands
func New(logger *zap.SugaredLogger, kubeContext string, runner Runner) *execer {
	return &execer{
		helmBinary:  command,
		logger:      logger,
		kubeContext: kubeContext,
		runner:      runner,
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
	helm.logger.Infof("Adding repo %v %v", name, repository)
	out, err := helm.exec(args, map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) UpdateRepo() error {
	helm.logger.Info("Updating repo")
	out, err := helm.exec([]string{"repo", "update"}, map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) UpdateDeps(chart string) error {
	helm.logger.Infof("Updating dependency %v", chart)
	out, err := helm.exec([]string{"dependency", "update", chart}, map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) BuildDeps(chart string) error {
	helm.logger.Infof("Building dependency %v", chart)
	out, err := helm.exec([]string{"dependency", "build", chart}, map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) SyncRelease(context HelmContext, name, chart string, flags ...string) error {
	helm.logger.Infof("Upgrading %v", chart)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "upgrade", "--install", "--reset-values", name, chart), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) ReleaseStatus(context HelmContext, name string, flags ...string) error {
	helm.logger.Infof("Getting status %v", name)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "status", name), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) List(context HelmContext, filter string, flags ...string) (string, error) {
	helm.logger.Infof("Listing releases matching %v", filter)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "list", filter), flags...), env)
	helm.write(out)
	return string(out), err
}

func (helm *execer) DecryptSecret(context HelmContext, name string, flags ...string) (string, error) {
	// Prevents https://github.com/roboll/helmfile/issues/258
	helm.decryptionMutex.Lock()
	defer helm.decryptionMutex.Unlock()

	absPath, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	helm.logger.Infof("Decrypting secret %v", absPath)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "secrets", "dec", absPath), flags...), env)
	helm.info(out)
	if err != nil {
		return "", err
	}

	tmpFile, err := ioutil.TempFile("", "secret")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// HELM_SECRETS_DEC_SUFFIX is used by the helm-secrets plugin to define the output file
	decSuffix := os.Getenv("HELM_SECRETS_DEC_SUFFIX")
	if len(decSuffix) == 0 {
		decSuffix = ".yaml.dec"
	}
	decFilename := strings.Replace(absPath, ".yaml", decSuffix, 1)

	// os.Rename seems to results in "cross-device link` errors in some cases
	// Instead of moving, copy it to the destination temp file as a work-around
	// See https://github.com/roboll/helmfile/issues/251#issuecomment-417166296f
	decFile, err := os.Open(decFilename)
	if err != nil {
		return "", err
	}
	defer decFile.Close()

	_, err = io.Copy(tmpFile, decFile)
	if err != nil {
		return "", err
	}

	if err := decFile.Close(); err != nil {
		return "", err
	}

	if err := os.Remove(decFilename); err != nil {
		return "", err
	}

	return tmpFile.Name(), err
}

func (helm *execer) TemplateRelease(chart string, flags ...string) error {
	out, err := helm.exec(append([]string{"template", chart}, flags...), map[string]string{})
	helm.write(out)
	return err
}

func (helm *execer) DiffRelease(context HelmContext, name, chart string, flags ...string) error {
	helm.logger.Infof("Comparing %v %v", name, chart)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "diff", "upgrade", "--reset-values", "--allow-unreleased", name, chart), flags...), env)
	// Do our best to write STDOUT only when diff existed
	// Unfortunately, this works only when you run helmfile with `--detailed-exitcode`
	detailedExitcodeEnabled := false
	for _, f := range flags {
		if strings.Contains(f, "detailed-exitcode") {
			detailedExitcodeEnabled = true
			break
		}
	}
	if detailedExitcodeEnabled {
		switch e := err.(type) {
		case ExitError:
			if e.ExitStatus() == 2 {
				helm.write(out)
				return err
			}
		}
	} else {
		helm.write(out)
	}
	return err
}

func (helm *execer) Lint(chart string, flags ...string) error {
	helm.logger.Infof("Linting %v", chart)
	out, err := helm.exec(append([]string{"lint", chart}, flags...), map[string]string{})
	helm.write(out)
	return err
}

func (helm *execer) Fetch(chart string, flags ...string) error {
	helm.logger.Infof("Fetching %v", chart)
	out, err := helm.exec(append([]string{"fetch", chart}, flags...), map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) DeleteRelease(context HelmContext, name string, flags ...string) error {
	helm.logger.Infof("Deleting %v", name)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "delete", name), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) TestRelease(context HelmContext, name string, flags ...string) error {
	helm.logger.Infof("Testing %v", name)
	preArgs := context.GetTillerlessArgs(helm.helmBinary)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "test", name), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) exec(args []string, env map[string]string) ([]byte, error) {
	cmdargs := args
	if len(helm.extra) > 0 {
		cmdargs = append(cmdargs, helm.extra...)
	}
	if helm.kubeContext != "" {
		cmdargs = append(cmdargs, "--kube-context", helm.kubeContext)
	}
	cmd := fmt.Sprintf("exec: %s %s", helm.helmBinary, strings.Join(cmdargs, " "))
	helm.logger.Debug(cmd)
	bytes, err := helm.runner.Execute(helm.helmBinary, cmdargs, env)
	helm.logger.Debugf("%s: %s", cmd, bytes)
	return bytes, err
}

func (helm *execer) info(out []byte) {
	if len(out) > 0 {
		helm.logger.Infof("%s", out)
	}
}

func (helm *execer) write(out []byte) {
	if len(out) > 0 {
		fmt.Printf("%s\n", out)
	}
}
