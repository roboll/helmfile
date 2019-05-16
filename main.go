package main

import (
	"fmt"
	"github.com/roboll/helmfile/args"
	"github.com/roboll/helmfile/cmd"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/pkg/app"
	"github.com/roboll/helmfile/state"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"strings"
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
	cliApp.Version = Version
	cliApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "helm-binary, b",
			Usage: "path to helm binary",
		},
		cli.StringFlag{
			Name:  "file, f",
			Usage: "load config from file or directory. defaults to `helmfile.yaml` or `helmfile.d`(means `helmfile.d/*.yaml`) in this preference",
		},
		cli.StringFlag{
			Name:  "environment, e",
			Usage: "specify the environment name. defaults to `default`",
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
			Name:  "interactive, i",
			Usage: "Request confirmation before attempting to modify clusters",
		},
	}

	cliApp.Before = configureLogging
	cliApp.Commands = []cli.Command{
		cmd.Deps(),
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
				return cmd.VisitAllDesiredStates(c, func(state *state.HelmState, helm helmexec.Interface, ctx app.Context) (bool, []error) {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					errs := ctx.SyncReposOnce(state, helm)

					ok := len(state.Repositories) > 0 && len(errs) == 0

					return ok, errs
				})
			},
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
				affectedReleases := state.AffectedReleases{}
				errs := findAndIterateOverDesiredStatesUsingFlags(c, func(st *state.HelmState, helm helmexec.Interface, _ app.Context) []error {
					return executeSyncCommand(c, &affectedReleases, st, helm)
				})
				affectedReleases.DisplayAffectedReleases(c.App.Metadata["logger"].(*zap.SugaredLogger))
				return errs
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
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
				cli.BoolFlag{
					Name:  "detailed-exitcode",
					Usage: "return a non-zero exit code when there are changes",
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
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface, ctx app.Context) []error {
					if !c.Bool("skip-deps") {
						if errs := ctx.SyncReposOnce(state, helm); errs != nil && len(errs) > 0 {
							return errs
						}
						if errs := state.BuildDeps(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}
					if errs := state.PrepareReleases(helm, "diff"); errs != nil && len(errs) > 0 {
						return errs
					}

					_, errs := executeDiffCommand(c, state, helm, c.Bool("detailed-exitcode"), c.Bool("suppress-secrets"))
					return errs
				})
			},
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
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface, ctx app.Context) []error {
					if !c.Bool("skip-deps") {
						if errs := ctx.SyncReposOnce(state, helm); errs != nil && len(errs) > 0 {
							return errs
						}
						if errs := state.BuildDeps(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}
					if errs := state.PrepareReleases(helm, "template"); errs != nil && len(errs) > 0 {
						return errs
					}
					return executeTemplateCommand(c, state, helm)
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
					Usage: "maximum number of concurrent downloads of release charts",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface, ctx app.Context) []error {
					values := c.StringSlice("values")
					args := args.GetArgs(c.String("args"), state)
					workers := c.Int("concurrency")
					if !c.Bool("skip-deps") {
						if errs := ctx.SyncReposOnce(state, helm); errs != nil && len(errs) > 0 {
							return errs
						}
						if errs := state.BuildDeps(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}
					if errs := state.PrepareReleases(helm, "lint"); errs != nil && len(errs) > 0 {
						return errs
					}
					return state.LintReleases(helm, values, args, workers)
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
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: func(c *cli.Context) error {
				affectedReleases := state.AffectedReleases{}
				errs := findAndIterateOverDesiredStatesUsingFlags(c, func(st *state.HelmState, helm helmexec.Interface, ctx app.Context) []error {
					if !c.Bool("skip-deps") {
						if errs := ctx.SyncReposOnce(st, helm); errs != nil && len(errs) > 0 {
							return errs
						}
						if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}
					if errs := st.PrepareReleases(helm, "sync"); errs != nil && len(errs) > 0 {
						return errs
					}
					return executeSyncCommand(c, &affectedReleases, st, helm)
				})
				affectedReleases.DisplayAffectedReleases(c.App.Metadata["logger"].(*zap.SugaredLogger))
				return errs
			},
		},
		{
			Name:  "apply",
			Usage: "apply all resources from state file only when there are changes",
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
				cli.BoolFlag{
					Name:  "suppress-secrets",
					Usage: "suppress secrets in the diff output. highly recommended to specify on CI/CD use-cases",
				},
				cli.BoolFlag{
					Name:  "skip-deps",
					Usage: "skip running `helm repo update` and `helm dependency build`",
				},
			},
			Action: func(c *cli.Context) error {
				affectedReleases := state.AffectedReleases{}
				errs := findAndIterateOverDesiredStatesUsingFlags(c, func(st *state.HelmState, helm helmexec.Interface, ctx app.Context) []error {
					if !c.Bool("skip-deps") {
						if errs := ctx.SyncReposOnce(st, helm); errs != nil && len(errs) > 0 {
							return errs
						}
						if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}
					if errs := st.PrepareReleases(helm, "apply"); errs != nil && len(errs) > 0 {
						return errs
					}

					releases, errs := executeDiffCommand(c, st, helm, true, c.Bool("suppress-secrets"))

					releasesToBeDeleted, err := st.DetectReleasesToBeDeleted(helm)
					if err != nil {
						errs = append(errs, err)
					}

					fatalErrs := []error{}

					noError := true
					for _, e := range errs {
						switch err := e.(type) {
						case *state.ReleaseError:
							if err.Code != 2 {
								noError = false
								fatalErrs = append(fatalErrs, e)
							}
						default:
							noError = false
							fatalErrs = append(fatalErrs, e)
						}
					}

					// sync only when there are changes
					if noError {
						if len(releases) == 0 && len(releasesToBeDeleted) == 0 {
							// TODO better way to get the logger
							logger := c.App.Metadata["logger"].(*zap.SugaredLogger)
							logger.Infof("")
							logger.Infof("No affected releases")
						} else {
							names := []string{}
							for _, r := range releases {
								names = append(names, fmt.Sprintf("  %s (%s) UPDATED", r.Name, r.Chart))
							}
							for _, r := range releasesToBeDeleted {
								names = append(names, fmt.Sprintf("  %s (%s) DELETED", r.Name, r.Chart))
							}

							msg := fmt.Sprintf(`Affected releases are:
%s

Do you really want to apply?
  Helmfile will apply all your changes, as shown above.

`, strings.Join(names, "\n"))
							interactive := c.GlobalBool("interactive")
							if !interactive || interactive && askForConfirmation(msg) {
								rs := []state.ReleaseSpec{}
								for _, r := range releases {
									rs = append(rs, *r)
								}
								for _, r := range releasesToBeDeleted {
									rs = append(rs, *r)
								}

								st.Releases = rs
								return executeSyncCommand(c, &affectedReleases, st, helm)
							}
						}
					}

					return fatalErrs
				})
				affectedReleases.DisplayAffectedReleases(c.App.Metadata["logger"].(*zap.SugaredLogger))
				return errs
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
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface, _ app.Context) []error {
					workers := c.Int("concurrency")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					return state.ReleaseStatuses(helm, workers)
				})
			},
		},
		{
			Name:  "delete",
			Usage: "DEPRECATED: delete releases from state file (helm delete)",
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
				affectedReleases := state.AffectedReleases{}
				errs := cmd.FindAndIterateOverDesiredStatesUsingFlagsWithReverse(c, true, func(state *state.HelmState, helm helmexec.Interface, _ app.Context) []error {
					purge := c.Bool("purge")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					names := make([]string, len(state.Releases))
					for i, r := range state.Releases {
						names[i] = fmt.Sprintf("  %s (%s)", r.Name, r.Chart)
					}

					msg := fmt.Sprintf(`Affected releases are:
%s

Do you really want to delete?
  Helmfile will delete all your releases, as shown above.

`, strings.Join(names, "\n"))
					interactive := c.GlobalBool("interactive")
					if !interactive || interactive && askForConfirmation(msg) {
						return state.DeleteReleases(&affectedReleases, helm, purge)
					}
					return nil
				})
				affectedReleases.DisplayAffectedReleases(c.App.Metadata["logger"].(*zap.SugaredLogger))
				return errs
			},
		},
		{
			Name:  "destroy",
			Usage: "deletes and then purges releases",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
			},
			Action: func(c *cli.Context) error {
				affectedReleases := state.AffectedReleases{}
				errs := cmd.FindAndIterateOverDesiredStatesUsingFlagsWithReverse(c, true, func(state *state.HelmState, helm helmexec.Interface, _ app.Context) []error {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					names := make([]string, len(state.Releases))
					for i, r := range state.Releases {
						names[i] = fmt.Sprintf("  %s (%s)", r.Name, r.Chart)
					}

					msg := fmt.Sprintf(`Affected releases are:
%s

Do you really want to delete?
  Helmfile will delete all your releases, as shown above.

`, strings.Join(names, "\n"))
					interactive := c.GlobalBool("interactive")
					if !interactive || interactive && askForConfirmation(msg) {
						return state.DeleteReleases(&affectedReleases, helm, true)
					}
					return nil
				})
				affectedReleases.DisplayAffectedReleases(c.App.Metadata["logger"].(*zap.SugaredLogger))
				return errs
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
				cli.IntFlag{
					Name:  "concurrency",
					Value: 0,
					Usage: "maximum number of concurrent helm processes to run, 0 is unlimited",
				},
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface, _ app.Context) []error {
					cleanup := c.Bool("cleanup")
					timeout := c.Int("timeout")
					concurrency := c.Int("concurrency")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					return state.TestReleases(helm, cleanup, timeout, concurrency)
				})
			},
		},
	}

	err := cliApp.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(3)
	}
}

func executeSyncCommand(c *cli.Context, affectedReleases *state.AffectedReleases, state *state.HelmState, helm helmexec.Interface) []error {
	args := args.GetArgs(c.String("args"), state)
	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	values := c.StringSlice("values")
	workers := c.Int("concurrency")

	return state.SyncReleases(affectedReleases, helm, values, workers)
}

func executeTemplateCommand(c *cli.Context, state *state.HelmState, helm helmexec.Interface) []error {
	args := args.GetArgs(c.String("args"), state)
	values := c.StringSlice("values")
	workers := c.Int("concurrency")

	return state.TemplateReleases(helm, values, args, workers)
}

func executeDiffCommand(c *cli.Context, st *state.HelmState, helm helmexec.Interface, detailedExitCode, suppressSecrets bool) ([]*state.ReleaseSpec, []error) {
	args := args.GetArgs(c.String("args"), st)
	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	values := c.StringSlice("values")
	workers := c.Int("concurrency")
	triggerCleanupEvents := c.Command.HasName("diff")

	return st.DiffReleases(helm, values, workers, detailedExitCode, suppressSecrets, triggerCleanupEvents)
}

func findAndIterateOverDesiredStatesUsingFlags(c *cli.Context, converge func(*state.HelmState, helmexec.Interface, app.Context) []error) error {
	return cmd.FindAndIterateOverDesiredStatesUsingFlagsWithReverse(c, false, converge)
}
