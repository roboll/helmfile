package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/roboll/helmfile/pkg/app/version"

	"github.com/roboll/helmfile/pkg/app"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/maputil"
	"github.com/roboll/helmfile/pkg/state"
	"github.com/urfave/cli"
	"go.uber.org/zap"
)

var logger *zap.SugaredLogger

func configureLogging(c *cli.Context) error {
	// Valid levels:
	// https://github.com/uber-go/zap/blob/7e7e266a8dbce911a49554b945538c5b950196b8/zapcore/level.go#L126
	logLevel := c.GlobalString("log-level")
	if c.GlobalBool("debug") {
		logLevel = "debug"
	} else if c.GlobalBool("quiet") {
		logLevel = "warn"
	}
	logger = helmexec.NewLogger(os.Stderr, logLevel)
	if c.App.Metadata == nil {
		// Auto-initialised in 1.19.0
		// https://github.com/urfave/cli/blob/master/CHANGELOG.md#1190---2016-11-19
		c.App.Metadata = make(map[string]interface{})
	}
	c.App.Metadata["logger"] = logger
	return nil
}

func main() {

	cliApp := cli.NewApp()
	cliApp.Name = "helmfile"
	cliApp.Usage = ""
	cliApp.Version = version.Version
	cliApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "helm-binary, b",
			Usage: "path to helm binary",
			Value: app.DefaultHelmBinary,
		},
		cli.StringFlag{
			Name:  "file, f",
			Usage: "load config from file or directory. defaults to `helmfile.yaml` or `helmfile.d`(means `helmfile.d/*.yaml`) in this preference",
		},
		cli.StringFlag{
			Name:  "environment, e",
			Usage: "specify the environment name. defaults to `default`",
		},
		cli.StringSliceFlag{
			Name:  "state-values-set",
			Usage: "set state values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)",
		},
		cli.StringSliceFlag{
			Name:  "state-values-file",
			Usage: "specify state values in a YAML file",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Silence output. Equivalent to log-level warn",
		},
		cli.StringFlag{
			Name:  "kube-context",
			Usage: "Set kubectl context. Uses current context by default",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable verbose output for Helm and set log-level to debug, this disables --quiet/-q effect",
		},
		cli.BoolFlag{
			Name:  "no-color",
			Usage: "Output without color",
		},
		cli.StringFlag{
			Name:  "log-level",
			Usage: "Set log level, default info",
		},
		cli.StringFlag{
			Name:  "namespace, n",
			Usage: "Set namespace. Uses the namespace set in the context by default, and is available in templates as {{ .Namespace }}",
		},
		cli.StringSliceFlag{
			Name: "selector, l",
			Usage: `Only run using the releases that match labels. Labels can take the form of foo=bar or foo!=bar.
	A release must match all labels in a group in order to be used. Multiple groups can be specified at once.
	--selector tier=frontend,tier!=proxy --selector tier=backend. Will match all frontend, non-proxy releases AND all backend releases.
	The name of a release can be used as a label. --selector name=myrelease`,
		},
		cli.BoolFlag{
			Name:  "allow-no-matching-release",
			Usage: `Do not exit with an error code if the provided selector has no matching releases.`,
		},
		cli.BoolFlag{
			Name:  "interactive, i",
			Usage: "Request confirmation before attempting to modify clusters",
		},
	}

	cliApp.Before = configureLogging
	cliApp.Commands = []cli.Command{
		{
			Name:  "deps",
			Usage: "update charts based on the contents of requirements.yaml",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.BoolFlag{
					Name:  "skip-repos",
					Usage: "skip running `helm repo update` before running `helm dependency build`",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Deps(c)
			}),
		},
		{
			Name:  "repos",
			Usage: "sync repositories from state file (helm repo add && helm repo update)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Repos(c)
			}),
		},
		{
			Name:  "charts",
			Usage: "DEPRECATED: sync releases from state file (helm upgrade --install)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.StringSliceFlag{
					Name:  "set",
					Usage: "additional values to be merged into the command",
				},
				cli.StringSliceFlag{
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.DeprecatedSyncCharts(c)
			}),
		},
		{
			Name:  "diff",
			Usage: "diff releases from state file against env (helm diff)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.StringSliceFlag{
					Name:  "set",
					Usage: "additional values to be merged into the command",
				},
				cli.StringSliceFlag{
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
				cli.BoolFlag{
					Name:  "detailed-exitcode",
					Usage: "return a non-zero exit code when there are changes",
				},
				cli.BoolFlag{
					Name:  "include-tests",
					Usage: "enable the diffing of the helm test hooks",
				},
				cli.BoolFlag{
					Name:  "suppress-secrets",
					Usage: "suppress secrets in the output. highly recommended to specify on CI/CD use-cases",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
				cli.IntFlag{
					Name:  "context",
					Value: 0,
					Usage: "output NUM lines of context around changes",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Diff(c)
			}),
		},
		{
			Name:  "template",
			Usage: "template releases from state file against env (helm template)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm template",
				},
				cli.StringSliceFlag{
					Name:  "set",
					Usage: "additional values to be merged into the command",
				},
				cli.StringSliceFlag{
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.StringFlag{
					Name:  "output-dir",
					Usage: "output directory to pass to helm template (helm template --output-dir)",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent downloads of release charts",
				},
				cli.BoolFlag{
					Name:  "validate",
					Usage: "validate your manifests against the Kubernetes cluster you are currently pointing at",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Template(c)
			}),
		},
		{
			Name:  "lint",
			Usage: "lint charts from state file (helm lint)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.StringSliceFlag{
					Name:  "set",
					Usage: "additional values to be merged into the command",
				},
				cli.StringSliceFlag{
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent downloads of release charts",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Lint(c)
			}),
		},
		{
			Name:  "sync",
			Usage: "sync all resources from state file (repos, releases and chart deps)",
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:  "set",
					Usage: "additional values to be merged into the command",
				},
				cli.StringSliceFlag{
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Sync(c)
			}),
		},
		{
			Name:  "apply",
			Usage: "apply all resources from state file only when there are changes",
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:  "set",
					Usage: "additional values to be merged into the command",
				},
				cli.StringSliceFlag{
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
				cli.IntFlag{
					Name:  "context",
					Value: 0,
					Usage: "output NUM lines of context around changes",
				},
				cli.BoolFlag{
					Name:  "detailed-exitcode",
					Usage: "return a non-zero exit code 2 instead of 0 when there were changes detected AND the changes are synced successfully",
				},
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.BoolFlag{
					Name:  "retain-values-files",
					Usage: "Stop cleaning up values files passed to Helm. Together with --log-level=debug, you can manually rerun helm commands as Helmfile did for debugging purpose",
				},
				cli.BoolFlag{
					Name:  "include-tests",
					Usage: "enable the diffing of the helm test hooks",
				},
				cli.BoolFlag{
					Name:  "suppress-secrets",
					Usage: "suppress secrets in the diff output. highly recommended to specify on CI/CD use-cases",
				},
				cli.BoolFlag{
					Name:  "suppress-diff",
					Usage: "suppress diff in the output. Usable in new installs",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Apply(c)
			}),
		},
		{
			Name:  "status",
			Usage: "retrieve status of releases in state file",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Status(c)
			}),
		},
		{
			Name:  "delete",
			Usage: "DEPRECATED: delete releases from state file (helm delete)",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.BoolFlag{
					Name:  "purge",
					Usage: "purge releases i.e. free release names and histories",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Delete(c)
			}),
		},
		{
			Name:  "destroy",
			Usage: "deletes and then purges releases",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Destroy(c)
			}),
		},
		{
			Name:  "test",
			Usage: "test releases from state file (helm test)",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "cleanup",
					Usage: "delete test pods upon completion",
				},
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass additional args to helm exec",
				},
				cli.IntFlag{
					Name:  "timeout",
					Value: 300,
					Usage: "maximum time for tests to run before being considered failed",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.Test(c)
			}),
		},
		{
			Name:  "build",
			Usage: "output compiled helmfile state(s) as YAML",
			Flags: []cli.Flag{},
			Action: action(func(run *app.App, c configImpl) error {
				return run.PrintState(c)
			}),
		},
		{
			Name:  "list",
			Usage: "list releases defined in state file",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "output",
					Value: "",
					Usage: "output releases list as a json string",
				},
			},
			Action: action(func(run *app.App, c configImpl) error {
				return run.ListReleases(c)
			}),
		},
	}

	err := cliApp.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
}

type configImpl struct {
	c *cli.Context

	set map[string]interface{}
}

func NewUrfaveCliConfigImpl(c *cli.Context) (configImpl, error) {
	if c.NArg() > 0 {
		cli.ShowAppHelp(c)
		return configImpl{}, fmt.Errorf("err: extraneous arguments: %s", strings.Join(c.Args(), ", "))
	}

	conf := configImpl{
		c: c,
	}

	optsSet := c.GlobalStringSlice("state-values-set")
	if len(optsSet) > 0 {
		set := map[string]interface{}{}
		for i := range optsSet {
			ops := strings.Split(optsSet[i], ",")
			for j := range ops {
				op := strings.SplitN(ops[j], "=", 2)
				k := maputil.ParseKey(op[0])
				v := op[1]

				maputil.Set(set, k, v)
			}
		}
		conf.set = set
	}

	return conf, nil
}

func (c configImpl) Set() []string {
	return c.c.StringSlice("set")
}

func (c configImpl) SkipRepos() bool {
	return c.c.Bool("skip-repos")
}

func (c configImpl) Values() []string {
	return c.c.StringSlice("values")
}

func (c configImpl) Args() string {
	args := c.c.String("args")
	enableHelmDebug := c.c.GlobalBool("debug")

	if enableHelmDebug {
		args = fmt.Sprintf("%s %s", args, "--debug")
	}
	return args
}

func (c configImpl) OutputDir() string {
	return c.c.String("output-dir")
}

func (c configImpl) Validate() bool {
	return c.c.Bool("validate")
}

func (c configImpl) Concurrency() int {
	return c.c.Int("concurrency")
}

func (c configImpl) HasCommandName(name string) bool {
	return c.c.Command.HasName(name)
}

// DiffConfig

func (c configImpl) SkipDeps() bool {
	return c.c.Bool("skip-deps")
}

func (c configImpl) DetailedExitcode() bool {
	return c.c.Bool("detailed-exitcode")
}

func (c configImpl) RetainValuesFiles() bool {
	return c.c.Bool("retain-values-files")
}

func (c configImpl) IncludeTests() bool {
	return c.c.Bool("include-tests")
}

func (c configImpl) SuppressSecrets() bool {
	return c.c.Bool("suppress-secrets")
}

func (c configImpl) SuppressDiff() bool {
	return c.c.Bool("suppress-diff")
}

// DeleteConfig

func (c configImpl) Purge() bool {
	return c.c.Bool("purge")
}

// TestConfig

func (c configImpl) Cleanup() bool {
	return c.c.Bool("cleanup")
}

func (c configImpl) Timeout() int {
	if !c.c.IsSet("timeout") {
		return state.EmptyTimeout
	}
	return c.c.Int("timeout")
}

// ListConfig

func (c configImpl) Output() string {
	return c.c.String("output")
}

// GlobalConfig

func (c configImpl) HelmBinary() string {
	return c.c.GlobalString("helm-binary")
}

func (c configImpl) KubeContext() string {
	return c.c.GlobalString("kube-context")
}

func (c configImpl) Namespace() string {
	return c.c.GlobalString("namespace")
}

func (c configImpl) FileOrDir() string {
	return c.c.GlobalString("file")
}

func (c configImpl) Selectors() []string {
	return c.c.GlobalStringSlice("selector")
}

func (c configImpl) StateValuesSet() map[string]interface{} {
	return c.set
}

func (c configImpl) StateValuesFiles() []string {
	return c.c.GlobalStringSlice("state-values-file")
}

func (c configImpl) Interactive() bool {
	return c.c.GlobalBool("interactive")
}

func (c configImpl) NoColor() bool {
	return c.c.GlobalBool("no-color")
}

func (c configImpl) Context() int {
	return c.c.Int("context")
}

func (c configImpl) Logger() *zap.SugaredLogger {
	return c.c.App.Metadata["logger"].(*zap.SugaredLogger)
}

func (c configImpl) Env() string {
	env := c.c.GlobalString("environment")
	if env == "" {
		env = state.DefaultEnv
	}
	return env
}

func action(do func(*app.App, configImpl) error) func(*cli.Context) error {
	return func(implCtx *cli.Context) error {
		conf, err := NewUrfaveCliConfigImpl(implCtx)
		if err != nil {
			return err
		}

		a := app.New(conf)

		if err := do(a, conf); err != nil {
			return toCliError(implCtx, err)
		}

		return nil
	}
}

func toCliError(c *cli.Context, err error) error {
	if err != nil {
		switch e := err.(type) {
		case *app.NoMatchingHelmfileError:
			noMatchingExitCode := 3
			if c.GlobalBool("allow-no-matching-release") {
				noMatchingExitCode = 0
			}
			return cli.NewExitError(e.Error(), noMatchingExitCode)
		case *app.Error:
			return cli.NewExitError(e.Error(), e.Code())
		default:
			panic(fmt.Errorf("BUG: please file an github issue for this unhandled error: %T: %v", e, e))
		}
	}
	return err
}
