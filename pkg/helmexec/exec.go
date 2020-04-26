package helmexec

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type decryptedSecret struct {
	mutex sync.RWMutex
	bytes []byte
}

type execer struct {
	helmBinary           string
	version              Version
	runner               Runner
	logger               *zap.SugaredLogger
	kubeContext          string
	extra                []string
	decryptedSecretMutex sync.Mutex
	decryptedSecrets     map[string]*decryptedSecret
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

func getHelmVersion(helmBinary string, logger *zap.SugaredLogger, runner Runner) Version {

	// Autodetect from `helm verison`
	bytes, err := runner.Execute(helmBinary, []string{"version", "--client", "--short"}, nil)
	if err != nil {
		panic(err)
	}

	if bytes == nil || len(bytes) == 0 {
		return Version{}
	}

	re := regexp.MustCompile("v(?P<major>\\d+)\\.(?P<minor>\\d+)\\.(?P<patch>\\d+)")
	matches := re.FindStringSubmatch(string(bytes))

	result := make(map[string]string)
	for i, name := range re.SubexpNames() {
		result[name] = matches[i]
	}

	major, err := strconv.Atoi(result["major"])
	if err != nil {
		panic(err)
	}

	minor, err := strconv.Atoi(result["minor"])
	if err != nil {
		panic(err)
	}

	patch, err := strconv.Atoi(result["patch"])
	if err != nil {
		panic(err)
	}

	// Support explicit helm3 opt-in via environment variable
	if os.Getenv("HELMFILE_HELM3") != "" && major < 3 {
		return Version{
			Major: 3,
			Minor: 0,
			Patch: 0,
		}
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
	}
}

// New for running helm commands
func New(helmBinary string, logger *zap.SugaredLogger, kubeContext string, runner Runner) *execer {
	return &execer{
		helmBinary:       helmBinary,
		version:          getHelmVersion(helmBinary, logger, runner),
		logger:           logger,
		kubeContext:      kubeContext,
		runner:           runner,
		decryptedSecrets: make(map[string]*decryptedSecret),
	}
}

func (helm *execer) SetExtraArgs(args ...string) {
	helm.extra = args
}

func (helm *execer) SetHelmBinary(bin string) {
	helm.helmBinary = bin
}

func (helm *execer) AddRepo(name, repository, cafile, certfile, keyfile, username, password string) error {
	var args []string
	if name == "" && repository != "" {
		helm.logger.Infof("empty field name\n")
		return fmt.Errorf("empty field name")
	}
	args = append(args, "repo", "add", name, repository)
	if certfile != "" && keyfile != "" {
		args = append(args, "--cert-file", certfile, "--key-file", keyfile)
	}
	if cafile != "" {
		args = append(args, "--ca-file", cafile)
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

func (helm *execer) BuildDeps(name, chart string) error {
	helm.logger.Infof("Building dependency release=%v, chart=%v", name, chart)
	out, err := helm.exec([]string{"dependency", "build", chart}, map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) UpdateDeps(chart string) error {
	helm.logger.Infof("Updating dependency %v", chart)
	out, err := helm.exec([]string{"dependency", "update", chart}, map[string]string{})
	helm.info(out)
	return err
}

func (helm *execer) SyncRelease(context HelmContext, name, chart string, flags ...string) error {
	helm.logger.Infof("Upgrading release=%v, chart=%v", name, chart)
	preArgs := context.GetTillerlessArgs(helm)
	env := context.getTillerlessEnv()

	if helm.IsHelm3() {
		flags = append(flags, "--history-max", strconv.Itoa(context.HistoryMax))
	} else {
		env["HELM_TILLER_HISTORY_MAX"] = strconv.Itoa(context.HistoryMax)
	}

	out, err := helm.exec(append(append(preArgs, "upgrade", "--install", "--reset-values", name, chart), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) ReleaseStatus(context HelmContext, name string, flags ...string) error {
	helm.logger.Infof("Getting status %v", name)
	preArgs := context.GetTillerlessArgs(helm)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "status", name), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) List(context HelmContext, filter string, flags ...string) (string, error) {
	helm.logger.Infof("Listing releases matching %v", filter)
	preArgs := context.GetTillerlessArgs(helm)
	env := context.getTillerlessEnv()
	var args []string
	if helm.IsHelm3() {
		args = []string{"list", "--filter", filter}
	} else {
		args = []string{"list", filter}
	}

	out, err := helm.exec(append(append(preArgs, args...), flags...), env)
	// In v2 we have been expecting `helm list FILTER` prints nothing.
	// In v3 helm still prints the header like `NAME	NAMESPACE	REVISION	UPDATED	STATUS	CHART	APP VERSION`,
	// which confuses helmfile's existing logic that treats any non-empty output from `helm list` is considered as the indication
	// of the release to exist.
	//
	// This fixes it by removing the header from the v3 output, so that the output is formatted the same as that of v2.
	if helm.IsHelm3() {
		lines := strings.Split(string(out), "\n")
		lines = lines[1:]
		out = []byte(strings.Join(lines, "\n"))
	}
	helm.write(out)
	return string(out), err
}

func (helm *execer) DecryptSecret(context HelmContext, name string, flags ...string) (string, error) {
	absPath, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}

	helm.logger.Debugf("Preparing to decrypt secret %v", absPath)
	helm.decryptedSecretMutex.Lock()

	secret, ok := helm.decryptedSecrets[absPath]

	// Cache miss
	if !ok {

		secret = &decryptedSecret{}
		helm.decryptedSecrets[absPath] = secret

		secret.mutex.Lock()
		defer secret.mutex.Unlock()
		helm.decryptedSecretMutex.Unlock()

		helm.logger.Infof("Decrypting secret %v", absPath)
		preArgs := context.GetTillerlessArgs(helm)
		env := context.getTillerlessEnv()
		out, err := helm.exec(append(append(preArgs, "secrets", "dec", absPath), flags...), env)
		helm.info(out)
		if err != nil {
			return "", err
		}

		// HELM_SECRETS_DEC_SUFFIX is used by the helm-secrets plugin to define the output file
		decSuffix := os.Getenv("HELM_SECRETS_DEC_SUFFIX")
		if len(decSuffix) == 0 {
			decSuffix = ".yaml.dec"
		}
		decFilename := strings.Replace(absPath, ".yaml", decSuffix, 1)

		secretBytes, err := ioutil.ReadFile(decFilename)
		if err != nil {
			return "", err
		}
		secret.bytes = secretBytes

		if err := os.Remove(decFilename); err != nil {
			return "", err
		}

	} else {
		// Cache hit
		helm.logger.Debugf("Found secret in cache %v", absPath)

		secret.mutex.RLock()
		helm.decryptedSecretMutex.Unlock()
		defer secret.mutex.RUnlock()
	}

	tmpFile, err := ioutil.TempFile("", "secret")
	if err != nil {
		return "", err
	}
	_, err = tmpFile.Write(secret.bytes)
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), err
}

func (helm *execer) TemplateRelease(name string, chart string, flags ...string) error {
	helm.logger.Infof("Templating release=%v, chart=%v", name, chart)
	var args []string
	if helm.IsHelm3() {
		args = []string{"template", name, chart}
	} else {
		args = []string{"template", chart, "--name", name}
	}

	out, err := helm.exec(append(args, flags...), map[string]string{})
	helm.write(out)
	return err
}

func (helm *execer) DiffRelease(context HelmContext, name, chart string, suppressDiff bool, flags ...string) error {
	helm.logger.Infof("Comparing release=%v, chart=%v", name, chart)
	preArgs := context.GetTillerlessArgs(helm)
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
				if !(suppressDiff) {
					helm.write(out)
				}
				return err
			}
		}
	} else if !(suppressDiff) {
		helm.write(out)
	}
	return err
}

func (helm *execer) Lint(name, chart string, flags ...string) error {
	helm.logger.Infof("Linting release=%v, chart=%v", name, chart)
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
	preArgs := context.GetTillerlessArgs(helm)
	env := context.getTillerlessEnv()
	out, err := helm.exec(append(append(preArgs, "delete", name), flags...), env)
	helm.write(out)
	return err
}

func (helm *execer) TestRelease(context HelmContext, name string, flags ...string) error {
	helm.logger.Infof("Testing %v", name)
	preArgs := context.GetTillerlessArgs(helm)
	env := context.getTillerlessEnv()
	args := []string{"test", name}
	out, err := helm.exec(append(append(preArgs, args...), flags...), env)
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

func (helm *execer) IsHelm3() bool {
	return helm.version.Major == 3
}

func (helm *execer) GetVersion() Version {
	return helm.version
}

func (helm *execer) IsVersionAtLeast(major int, minor int) bool {
	return helm.version.Major >= major && helm.version.Minor >= minor
}
