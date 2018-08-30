package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"os/exec"

	"github.com/roboll/helmfile/args"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"github.com/roboll/helmfile/tmpl"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	DefaultHelmfile          = "helmfile.yaml"
	DeprecatedHelmfile       = "charts.yaml"
	DefaultHelmfileDirectory = "helmfile.d"
)

var Version string

var logger *zap.SugaredLogger

func configureLogging(c *cli.Context) error {
	// Valid levels:
	// https://github.com/uber-go/zap/blob/7e7e266a8dbce911a49554b945538c5b950196b8/zapcore/level.go#L126
	logLevel := c.GlobalString("log-level")
	if c.GlobalBool("quiet") {
		logLevel = "warn"
	}
	var level zapcore.Level
	err := level.Set(logLevel)
	if err != nil {
		return err
	}
	logger = helmexec.NewLogger(os.Stdout, logLevel)
	if c.App.Metadata == nil {
		// Auto-initialised in 1.19.0
		// https://github.com/urfave/cli/blob/master/CHANGELOG.md#1190---2016-11-19
		c.App.Metadata = make(map[string]interface{})
	}
	c.App.Metadata["logger"] = logger
	return nil
}

func main() {

	app := cli.NewApp()
	app.Name = "helmfile"
	app.Usage = ""
	app.Version = Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "helm-binary, b",
			Usage: "path to helm binary",
		},
		cli.StringFlag{
			Name:  "file, f",
			Usage: "load config from file or directory. defaults to `helmfile.yaml` or `helmfile.d`(means `helmfile.d/*.yaml`) in this preference",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Silence output. Equivalent to log-level warn",
		},
		cli.StringFlag{
			Name:  "kube-context",
			Usage: "Set kubectl context. Uses current context by default",
		},
		cli.StringFlag{
			Name:  "log-level",
			Usage: "Set log level, default info",
		},
		cli.StringFlag{
			Name:  "namespace, n",
			Usage: "Set namespace. Uses the namespace set in the context by default",
		},
		cli.StringSliceFlag{
			Name: "selector, l",
			Usage: `Only run using the releases that match labels. Labels can take the form of foo=bar or foo!=bar.
	A release must match all labels in a group in order to be used. Multiple groups can be specified at once.
	--selector tier=frontend,tier!=proxy --selector tier=backend. Will match all frontend, non-proxy releases AND all backend releases.
	The name of a release can be used as a label. --selector name=myrelease`,
		},
	}

	app.Before = configureLogging
	app.Commands = []cli.Command{
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
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					return state.SyncRepos(helm)
				})
			},
		},
		{
			Name:  "charts",
			Usage: "sync releases from state file (helm upgrade --install)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
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
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					values := c.StringSlice("values")
					workers := c.Int("concurrency")

					return state.SyncReleases(helm, values, workers)
				})
			},
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
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.BoolFlag{
					Name:  "sync-repos",
					Usage: "enable a repo sync prior to diffing",
				},
				cli.BoolFlag{
					Name:  "detailed-exitcode",
					Usage: "return a non-zero exit code when there are changes",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
			},
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					if c.Bool("sync-repos") {
						if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}

					values := c.StringSlice("values")
					workers := c.Int("concurrency")
					detailedExitCode := c.Bool("detailed-exitcode")

					return state.DiffReleases(helm, values, workers, detailedExitCode)
				})
			},
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
					Name:  "values",
					Usage: "additional value files to be merged into the command",
				},
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
			},
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					values := c.StringSlice("values")
					workers := c.Int("concurrency")

					return state.LintReleases(helm, values, workers)
				})
			},
		},
		{
			Name:  "sync",
			Usage: "sync all resources from state file (repos, releases and chart deps)",
			Flags: []cli.Flag{
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
			},
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
						return errs
					}

					if errs := state.UpdateDeps(helm); errs != nil && len(errs) > 0 {
						return errs
					}

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					values := c.StringSlice("values")
					workers := c.Int("concurrency")

					return state.SyncReleases(helm, values, workers)
				})
			},
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
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					workers := c.Int("concurrency")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					return state.ReleaseStatuses(helm, workers)
				})
			},
		},
		{
			Name:  "delete",
			Usage: "delete releases from state file (helm delete)",
			Flags: []cli.Flag{
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
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					purge := c.Bool("purge")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					return state.DeleteReleases(helm, purge)
				})
			},
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
			},
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					cleanup := c.Bool("cleanup")
					timeout := c.Int("timeout")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}
					if c.GlobalString("helm-binary") != "" {
						helm.SetHelmBinary(c.GlobalString("helm-binary"))
					}

					return state.TestReleases(helm, cleanup, timeout)
				})
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
}

func eachDesiredStateDo(c *cli.Context, converge func(*state.HelmState, helmexec.Interface) []error) error {
	fileOrDirPath := c.GlobalString("file")
	desiredStateFiles, err := findDesiredStateFiles(fileOrDirPath)
	if err != nil {
		return err
	}
	allSelectorNotMatched := true
	for _, f := range desiredStateFiles {
		yamlBuf, err := tmpl.RenderTemplateFileToBuffer(f)
		if err != nil {
			return err
		}
		state, helm, noReleases, err := loadDesiredStateFromFile(
			yamlBuf.Bytes(),
			f,
			c.GlobalString("kube-context"),
			c.GlobalString("namespace"),
			c.GlobalStringSlice("selector"),
			c.App.Metadata["logger"].(*zap.SugaredLogger),
		)
		if err != nil {
			return err
		}
		allSelectorNotMatched = allSelectorNotMatched && noReleases
		if noReleases {
			continue
		}
		errs := converge(state, helm)
		if err := clean(state, errs); err != nil {
			return err
		}
	}
	if allSelectorNotMatched {
		logger.Error("specified selector did not match any releases in any helmfile")
		os.Exit(2)
	}
	return nil
}

func findDesiredStateFiles(specifiedPath string) ([]string, error) {
	var helmfileDir string
	if specifiedPath != "" {
		if fileExistsAt(specifiedPath) {
			return []string{specifiedPath}, nil
		} else if directoryExistsAt(specifiedPath) {
			helmfileDir = specifiedPath
		} else {
			return []string{}, fmt.Errorf("specified state file %s is not found", specifiedPath)
		}
	} else {
		var defaultFile string
		if fileExistsAt(DefaultHelmfile) {
			defaultFile = DefaultHelmfile
		} else if fileExistsAt(DeprecatedHelmfile) {
			log.Printf(
				"warn: %s is being loaded: %s is deprecated in favor of %s. See https://github.com/roboll/helmfile/issues/25 for more information",
				DeprecatedHelmfile,
				DeprecatedHelmfile,
				DefaultHelmfile,
			)
			defaultFile = DeprecatedHelmfile
		}

		if directoryExistsAt(DefaultHelmfileDirectory) {
			if defaultFile != "" {
				return []string{}, fmt.Errorf("configuration conlict error: you can have either %s or %s, but not both", defaultFile, DefaultHelmfileDirectory)
			}

			helmfileDir = DefaultHelmfileDirectory
		} else if defaultFile != "" {
			return []string{defaultFile}, nil
		} else {
			return []string{}, fmt.Errorf("no state file found. It must be named %s/*.yaml, %s, or %s, or otherwise specified with the --file flag", DefaultHelmfileDirectory, DefaultHelmfile, DeprecatedHelmfile)
		}
	}

	files, err := filepath.Glob(filepath.Join(helmfileDir, "*.yaml"))
	if err != nil {
		return []string{}, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i] < files[j]
	})
	return files, nil
}

func fileExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsRegular()
}

func directoryExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsDir()
}

func loadDesiredStateFromFile(yaml []byte, file string, kubeContext, namespace string, labels []string, logger *zap.SugaredLogger) (*state.HelmState, helmexec.Interface, bool, error) {
	st, err := state.CreateFromYaml(yaml, file, logger)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to read %s: %v", file, err)
	}

	if st.Context != "" {
		if kubeContext != "" {
			log.Printf("err: Cannot use option --kube-context and set attribute context.")
			os.Exit(1)
		}

		kubeContext = st.Context
	}
	if namespace != "" {
		if st.Namespace != "" {
			log.Printf("err: Cannot use option --namespace and set attribute namespace.")
			os.Exit(1)
		}
		st.Namespace = namespace
	}

	if len(labels) > 0 {
		err = st.FilterReleases(labels)
		if err != nil {
			log.Print(err)
			return nil, nil, true, nil
		}
	}

	releaseNameCounts := map[string]int{}
	for _, r := range st.Releases {
		releaseNameCounts[r.Name] += 1
	}
	for name, c := range releaseNameCounts {
		if c > 1 {
			return nil, nil, false, fmt.Errorf("duplicate release \"%s\" found: there were %d releases named \"%s\" matching specified selector", name, c, name)
		}
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		errs := []error{fmt.Errorf("Received [%s] to shutdown ", sig)}
		clean(st, errs)
	}()

	return st, helmexec.New(logger, kubeContext), len(st.Releases) == 0, nil
}

func clean(st *state.HelmState, errs []error) error {
	if errs == nil {
		errs = []error{}
	}

	cleanErrs := st.Clean()
	if cleanErrs != nil {
		errs = append(errs, cleanErrs...)
	}

	if errs != nil && len(errs) > 0 {
		for _, err := range errs {
			switch e := err.(type) {
			case *state.ReleaseError:
				fmt.Printf("err: release \"%s\" in \"%s\" failed: %v\n", e.Name, st.FilePath, e)
			default:
				fmt.Printf("err: %v\n", e)
			}
		}
		switch e := errs[0].(type) {
		case *exec.ExitError:
			// Propagate any non-zero exit status from the external command like `helm` that is failed under the hood
			status := e.Sys().(syscall.WaitStatus)
			os.Exit(status.ExitStatus())
		default:
			os.Exit(1)
		}
	}
	return nil
}
