package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"github.com/urfave/cli"
	"path/filepath"
	"sort"
)

const (
	DefaultHelmfile          = "helmfile.yaml"
	DeprecatedHelmfile       = "charts.yaml"
	DefaultHelmfileDirectory = "helmfile.d"
)

var Version string

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
			Value: DefaultHelmfile,
			Usage: "load config from `FILE`",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "silence output",
		},
		cli.StringFlag{
			Name:  "kube-context",
			Usage: "Set kubectl context. Uses current context by default",
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
					args := getArgs(c, state)
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
			Usage: "sync charts from state file (helm upgrade --install)",
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
					args := getArgs(c, state)
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
			Usage: "diff charts from state file against env (helm diff)",
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
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
			},
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					args := getArgs(c, state)
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

					return state.DiffReleases(helm, values, workers)
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
					args := c.String("args")
					if len(args) > 0 {
						helm.SetExtraArgs(strings.Split(args, " ")...)
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
			Usage: "sync all resources from state file (repos, charts and local chart deps)",
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

					args := getArgs(c, state)
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

					args := getArgs(c, state)
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
				cli.BoolFlag{
					Name:  "purge",
					Usage: "purge releases i.e. free release names and histories",
				},
			},
			Action: func(c *cli.Context) error {
				return eachDesiredStateDo(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					purge := c.Bool("purge")

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

					args := getArgs(c, state)
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
		log.Printf("err: %s", err.Error())
		os.Exit(1)
	}
}

func eachDesiredStateDo(c *cli.Context, converge func(*state.HelmState, helmexec.Interface) []error) error {
	fileOrDirPath := c.GlobalString("file")
	desiredStateFiles, err := findDesiredStateFiles(fileOrDirPath)
	if err != nil {
		return err
	}
	for _, f := range desiredStateFiles {
		state, helm, err := loadDesiredStateFromFile(c, f)
		if err != nil {
			return err
		}
		errs := converge(state, helm)
		if err := clean(state, errs); err != nil {
			return err
		}
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

func loadDesiredStateFromFile(c *cli.Context, file string) (*state.HelmState, helmexec.Interface, error) {
	quiet := c.GlobalBool("quiet")
	kubeContext := c.GlobalString("kube-context")
	namespace := c.GlobalString("namespace")
	labels := c.GlobalStringSlice("selector")

	st, err := state.ReadFromFile(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read %s: %v", file, err)
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
			os.Exit(1)
		}
	}
	var writer io.Writer
	if !quiet {
		writer = os.Stdout
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		errs := []error{fmt.Errorf("Received [%s] to shutdown ", sig)}
		clean(st, errs)
	}()

	return st, helmexec.New(writer, kubeContext), nil
}

func clean(state *state.HelmState, errs []error) error {
	if errs == nil {
		errs = []error{}
	}

	cleanErrs := state.Clean()
	if cleanErrs != nil {
		errs = append(errs, cleanErrs...)
	}

	if errs != nil && len(errs) > 0 {
		for _, err := range errs {
			fmt.Printf("err: %s\n", err.Error())
		}
		os.Exit(1)
	}
	return nil
}

func getArgs(c *cli.Context, state *state.HelmState) []string {
	args := c.String("args")
	if len(args) > 0 {
		state.HelmDefaults.Args = strings.Split(args, " ")
	}

	return state.HelmDefaults.Args
}
