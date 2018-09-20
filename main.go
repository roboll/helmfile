package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"io/ioutil"

	"strings"

	"github.com/roboll/helmfile/args"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/state"
	"github.com/roboll/helmfile/tmpl"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os/exec"
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
				return visitAllDesiredStates(c, func(state *state.HelmState, helm helmexec.Interface) (bool, []error) {
					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
					}

					errs := state.SyncRepos(helm)

					ok := len(state.Repositories) > 0 && len(errs) == 0

					return ok, errs
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
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					return executeSyncCommand(c, state, helm)
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
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
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
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
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
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					values := c.StringSlice("values")
					args := args.GetArgs(c.String("args"), state)
					workers := c.Int("concurrency")
					if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
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
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
						return errs
					}
					if errs := state.UpdateDeps(helm); errs != nil && len(errs) > 0 {
						return errs
					}
					return executeSyncCommand(c, state, helm)
				})
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
					Name:  "auto-approve",
					Usage: "Skip interactive approval before applying",
				},
				cli.BoolFlag{
					Name:  "suppress-secrets",
					Usage: "suppress secrets in the diff output. highly recommended to specify on CI/CD use-cases",
				},
				cli.BoolFlag{
					Name:  "skip-repo-update",
					Usage: "skip running `helm repo update` on repositories declared in helmfile",
				},
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlags(c, func(st *state.HelmState, helm helmexec.Interface) []error {
					if !c.Bool("skip-repo-update") {
						if errs := st.SyncRepos(helm); errs != nil && len(errs) > 0 {
							return errs
						}
					}
					if errs := st.UpdateDeps(helm); errs != nil && len(errs) > 0 {
						return errs
					}

					releases, errs := executeDiffCommand(c, st, helm, true, c.Bool("suppress-secrets"))

					releasesToBeDeleted, err := st.DetectReleasesToBeDeleted(helm)
					if err != nil {
						errs = append(errs, err)
					}

					noError := true
					for _, e := range errs {
						switch err := e.(type) {
						case *state.DiffError:
							noError = noError && err.Code == 2
						default:
							noError = false
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
							autoApprove := c.Bool("auto-approve")
							if autoApprove || !autoApprove && askForConfirmation(msg) {
								rs := []state.ReleaseSpec{}
								for _, r := range releases {
									rs = append(rs, *r)
								}
								for _, r := range releasesToBeDeleted {
									rs = append(rs, *r)
								}

								st.Releases = rs
								return executeSyncCommand(c, st, helm)
							}
						}
					}

					return errs
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
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
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
			Usage: "delete releases from state file (helm delete)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "args",
					Value: "",
					Usage: "pass args to helm exec",
				},
				cli.BoolFlag{
					Name:  "auto-approve",
					Usage: "Skip interactive approval before deleting",
				},
				cli.BoolFlag{
					Name:  "purge",
					Usage: "purge releases i.e. free release names and histories",
				},
			},
			Action: func(c *cli.Context) error {
				return findAndIterateOverDesiredStatesUsingFlagsWithReverse(c, true, func(state *state.HelmState, helm helmexec.Interface) []error {
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
					autoApprove := c.Bool("auto-approve")
					if autoApprove || !autoApprove && askForConfirmation(msg) {
						return state.DeleteReleases(helm, purge)
					}
					return nil
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
				return findAndIterateOverDesiredStatesUsingFlags(c, func(state *state.HelmState, helm helmexec.Interface) []error {
					cleanup := c.Bool("cleanup")
					timeout := c.Int("timeout")

					args := args.GetArgs(c.String("args"), state)
					if len(args) > 0 {
						helm.SetExtraArgs(args...)
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

func executeSyncCommand(c *cli.Context, state *state.HelmState, helm helmexec.Interface) []error {
	args := args.GetArgs(c.String("args"), state)
	if len(args) > 0 {
		helm.SetExtraArgs(args...)
	}

	values := c.StringSlice("values")
	workers := c.Int("concurrency")

	return state.SyncReleases(helm, values, workers)
}

func executeTemplateCommand(c *cli.Context, state *state.HelmState, helm helmexec.Interface) []error {
	if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
		return errs
	}

	if errs := state.UpdateDeps(helm); errs != nil && len(errs) > 0 {
		return errs
	}

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

	if c.Bool("sync-repos") {
		if errs := st.SyncRepos(helm); errs != nil && len(errs) > 0 {
			return []*state.ReleaseSpec{}, errs
		}
	}

	values := c.StringSlice("values")
	workers := c.Int("concurrency")

	return st.DiffReleases(helm, values, workers, detailedExitCode, suppressSecrets)
}

type app struct {
	kubeContext       string
	logger            *zap.SugaredLogger
	readFile          func(string) ([]byte, error)
	glob              func(string) ([]string, error)
	abs               func(string) (string, error)
	fileExistsAt      func(string) bool
	directoryExistsAt func(string) bool
	reverse           bool
	env               string
	namespace         string
	selectors         []string
}

func findAndIterateOverDesiredStatesUsingFlags(c *cli.Context, converge func(*state.HelmState, helmexec.Interface) []error) error {
	return findAndIterateOverDesiredStatesUsingFlagsWithReverse(c, false, converge)
}

func initAppEntry(c *cli.Context, reverse bool) (*app, string, error) {
	if c.NArg() > 0 {
		cli.ShowAppHelp(c)
		return nil, "", fmt.Errorf("err: extraneous arguments: %s", strings.Join(c.Args(), ", "))
	}

	fileOrDir := c.GlobalString("file")
	kubeContext := c.GlobalString("kube-context")
	namespace := c.GlobalString("namespace")
	selectors := c.GlobalStringSlice("selector")
	logger := c.App.Metadata["logger"].(*zap.SugaredLogger)

	env := c.GlobalString("environment")
	if env == "" {
		env = state.DefaultEnv
	}

	app := &app{
		readFile:          ioutil.ReadFile,
		glob:              filepath.Glob,
		abs:               filepath.Abs,
		fileExistsAt:      fileExistsAt,
		directoryExistsAt: directoryExistsAt,
		kubeContext:       kubeContext,
		logger:            logger,
		reverse:           reverse,
		env:               env,
		namespace:         namespace,
		selectors:         selectors,
	}

	return app, fileOrDir, nil
}

func visitAllDesiredStates(c *cli.Context, converge func(*state.HelmState, helmexec.Interface) (bool, []error)) error {
	app, fileOrDir, err := initAppEntry(c, false)
	if err != nil {
		return err
	}

	convergeWithHelmBinary := func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
		if c.GlobalString("helm-binary") != "" {
			helm.SetHelmBinary(c.GlobalString("helm-binary"))
		}
		return converge(st, helm)
	}

	err = app.VisitDesiredStates(fileOrDir, convergeWithHelmBinary)

	return toCliError(err)
}

func toCliError(err error) error {
	if err != nil {
		switch e := err.(type) {
		case *noMatchingHelmfileError:
			return cli.NewExitError(e.Error(), 2)
		case *exec.ExitError:
			// Propagate any non-zero exit status from the external command like `helm` that is failed under the hood
			status := e.Sys().(syscall.WaitStatus)
			return cli.NewExitError(e.Error(), status.ExitStatus())
		case *state.DiffError:
			return cli.NewExitError(e.Error(), e.Code)
		default:
			return cli.NewExitError(e.Error(), 1)
		}
	}
	return err
}

func findAndIterateOverDesiredStatesUsingFlagsWithReverse(c *cli.Context, reverse bool, converge func(*state.HelmState, helmexec.Interface) []error) error {
	app, fileOrDir, err := initAppEntry(c, reverse)
	if err != nil {
		return err
	}

	convergeWithHelmBinary := func(st *state.HelmState, helm helmexec.Interface) []error {
		if c.GlobalString("helm-binary") != "" {
			helm.SetHelmBinary(c.GlobalString("helm-binary"))
		}
		return converge(st, helm)
	}

	err = app.VisitDesiredStatesWithReleasesFiltered(fileOrDir, convergeWithHelmBinary)

	return toCliError(err)
}

type noMatchingHelmfileError struct {
	selectors []string
	env       string
}

func (e *noMatchingHelmfileError) Error() string {
	return fmt.Sprintf(
		"err: no releases found that matches specified selector(%s) and environment(%s), in any helmfile",
		strings.Join(e.selectors, ", "),
		e.env,
	)
}

func prependLineNumbers(text string) string {
	buf := bytes.NewBufferString("")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		buf.WriteString(fmt.Sprintf("%2d: %s\n", i, line))
	}
	return buf.String()
}

type twoPassRenderer struct {
	reader    func(string) ([]byte, error)
	env       string
	namespace string
	filename  string
	logger    *zap.SugaredLogger
	abs       func(string) (string, error)
}

func (r *twoPassRenderer) renderEnvironment(content []byte) environment.Environment {
	firstPassEnv := environment.Environment{Name: r.env, Values: map[string]interface{}(nil)}
	firstPassRenderer := tmpl.NewFirstPassRenderer(filepath.Dir(r.filename), firstPassEnv)

	// parse as much as we can, tolerate errors, this is a preparse
	yamlBuf, err := firstPassRenderer.RenderTemplateContentToBuffer(content)
	if err != nil && logger != nil {
		r.logger.Debugf("first-pass rendering input of \"%s\":\n%s", r.filename, prependLineNumbers(string(content)))
	}
	c := state.NewCreator(r.logger, r.reader, r.abs)
	c.Strict = false
	// create preliminary state, as we may have an environment. Tolerate errors.
	prestate, err := c.CreateFromYaml(yamlBuf.Bytes(), r.filename, r.env)
	if err != nil && r.logger != nil {
		switch err.(type) {
		case *state.StateLoadError:
			r.logger.Infof("could not deduce `environment:` block, configuring only .Environment.Name. error: %v", err)
		}
		r.logger.Debugf("error in first-pass rendering: result of \"%s\":\n%s", r.filename, prependLineNumbers(yamlBuf.String()))
	}
	if prestate != nil {
		firstPassEnv = prestate.Env
	}
	return firstPassEnv
}

func (r *twoPassRenderer) renderTemplate(content []byte) (*bytes.Buffer, error) {
	// try a first pass render. This will always succeed, but can produce a limited env
	firstPassEnv := r.renderEnvironment(content)

	secondPassRenderer := tmpl.NewFileRenderer(r.reader, filepath.Dir(r.filename), firstPassEnv, r.namespace)
	yamlBuf, err := secondPassRenderer.RenderTemplateContentToBuffer(content)
	if err != nil {
		if r.logger != nil {
			r.logger.Debugf("second-pass rendering failed, input of \"%s\":\n%s", r.filename, prependLineNumbers(string(content)))
		}
		return nil, err
	}
	if r.logger != nil {
		r.logger.Debugf("second-pass rendering result of \"%s\":\n%s", r.filename, prependLineNumbers(yamlBuf.String()))
	}
	return yamlBuf, nil
}

func (a *app) VisitDesiredStates(fileOrDir string, converge func(*state.HelmState, helmexec.Interface) (bool, []error)) error {
	desiredStateFiles, err := a.findDesiredStateFiles(fileOrDir)
	if err != nil {
		return err
	}

	noMatchInHelmfiles := true
	for _, f := range desiredStateFiles {
		a.logger.Debugf("Processing %s", f)

		content, err := a.readFile(f)
		if err != nil {
			return err
		}
		// render template, in two runs
		r := &twoPassRenderer{
			reader:    a.readFile,
			env:       a.env,
			namespace: a.namespace,
			filename:  f,
			logger:    a.logger,
			abs:       a.abs,
		}
		yamlBuf, err := r.renderTemplate(content)
		if err != nil {
			return fmt.Errorf("error during %s parsing: %v", f, err)
		}

		st, err := a.loadDesiredStateFromYaml(
			yamlBuf.Bytes(),
			f,
			a.namespace,
			a.env,
		)

		helm := helmexec.New(a.logger, a.kubeContext)

		if err != nil {
			switch stateLoadErr := err.(type) {
			// Addresses https://github.com/roboll/helmfile/issues/279
			case *state.StateLoadError:
				switch stateLoadErr.Cause.(type) {
				case *state.UndefinedEnvError:
					continue
				default:
					return err
				}
			default:
				return err
			}
		}

		errs := []error{}

		if len(st.Helmfiles) > 0 {
			noMatchInSubHelmfiles := true
			for _, m := range st.Helmfiles {
				if err := a.VisitDesiredStates(m, converge); err != nil {
					switch err.(type) {
					case *noMatchingHelmfileError:

					default:
						return fmt.Errorf("failed processing %s: %v", m, err)
					}
				} else {
					noMatchInSubHelmfiles = false
				}
			}
			noMatchInHelmfiles = noMatchInHelmfiles && noMatchInSubHelmfiles
		} else {
			var processed bool
			processed, errs = converge(st, helm)
			noMatchInHelmfiles = noMatchInHelmfiles && !processed
		}

		if err := clean(st, errs); err != nil {
			return err
		}
	}
	if noMatchInHelmfiles {
		return &noMatchingHelmfileError{selectors: a.selectors, env: a.env}
	}
	return nil
}

func (a *app) VisitDesiredStatesWithReleasesFiltered(fileOrDir string, converge func(*state.HelmState, helmexec.Interface) []error) error {
	selectors := a.selectors

	err := a.VisitDesiredStates(fileOrDir, func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
		if len(selectors) > 0 {
			err := st.FilterReleases(selectors)
			if err != nil {
				return false, []error{err}
			}
		}

		releaseNameCounts := map[string]int{}
		for _, r := range st.Releases {
			releaseNameCounts[r.Name]++
		}
		for name, c := range releaseNameCounts {
			if c > 1 {
				return false, []error{fmt.Errorf("duplicate release \"%s\" found: there were %d releases named \"%s\" matching specified selector", name, c, name)}
			}
		}

		errs := converge(st, helm)

		processed := len(st.Releases) != 0 && len(errs) == 0

		return processed, errs
	})
	if err != nil {
		return err
	}
	return nil
}

func (a *app) findDesiredStateFiles(specifiedPath string) ([]string, error) {
	var helmfileDir string
	if specifiedPath != "" {
		if a.fileExistsAt(specifiedPath) {
			return []string{specifiedPath}, nil
		} else if a.directoryExistsAt(specifiedPath) {
			helmfileDir = specifiedPath
		} else {
			return []string{}, fmt.Errorf("specified state file %s is not found", specifiedPath)
		}
	} else {
		var defaultFile string
		if a.fileExistsAt(DefaultHelmfile) {
			defaultFile = DefaultHelmfile
		} else if a.fileExistsAt(DeprecatedHelmfile) {
			log.Printf(
				"warn: %s is being loaded: %s is deprecated in favor of %s. See https://github.com/roboll/helmfile/issues/25 for more information",
				DeprecatedHelmfile,
				DeprecatedHelmfile,
				DefaultHelmfile,
			)
			defaultFile = DeprecatedHelmfile
		}

		if a.directoryExistsAt(DefaultHelmfileDirectory) {
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

	files, err := a.glob(filepath.Join(helmfileDir, "*.yaml"))
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

func (a *app) loadDesiredStateFromYaml(yaml []byte, file string, namespace string, env string) (*state.HelmState, error) {
	c := state.NewCreator(a.logger, a.readFile, a.abs)
	st, err := c.CreateFromYaml(yaml, file, env)
	if err != nil {
		return nil, err
	}

	helmfiles := []string{}
	for _, globPattern := range st.Helmfiles {
		helmfileRelativePattern := st.JoinBase(globPattern)
		matches, err := a.glob(helmfileRelativePattern)
		if err != nil {
			return nil, fmt.Errorf("failed processing %s: %v", globPattern, err)
		}
		sort.Strings(matches)

		helmfiles = append(helmfiles, matches...)
	}
	st.Helmfiles = helmfiles

	if a.reverse {
		rev := func(i, j int) bool {
			return j < i
		}
		sort.Slice(st.Releases, rev)
		sort.Slice(st.Helmfiles, rev)
	}

	if a.kubeContext != "" {
		if st.Context != "" {
			log.Printf("err: Cannot use option --kube-context and set attribute context.")
			os.Exit(1)
		}
		st.Context = a.kubeContext
	}
	if namespace != "" {
		if st.Namespace != "" {
			log.Printf("err: Cannot use option --namespace and set attribute namespace.")
			os.Exit(1)
		}
		st.Namespace = namespace
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		errs := []error{fmt.Errorf("Received [%s] to shutdown ", sig)}
		clean(st, errs)
	}()

	return st, nil
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
		return errs[0]
	}
	return nil
}
