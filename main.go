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
)

const (
	DefaultHelmfile    = "helmfile.yaml"
	DeprecatedHelmfile = "charts.yaml"
)

var Version string

func main() {

	app := cli.NewApp()
	app.Name = "helmfile"
	app.Usage = ""
	app.Version = Version
	app.Flags = []cli.Flag{
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
				state, helm, err := before(c)
				if err != nil {
					return err
				}

				args := c.String("args")
				if len(args) > 0 {
					helm.SetExtraArgs(strings.Split(args, " ")...)
				}

				errs := state.SyncRepos(helm)
				return clean(state, errs)
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
				state, helm, err := before(c)
				if err != nil {
					return err
				}

				args := c.String("args")
				if len(args) > 0 {
					helm.SetExtraArgs(strings.Split(args, " ")...)
				}

				values := c.StringSlice("values")
				workers := c.Int("concurrency")

				errs := state.SyncReleases(helm, values, workers)
				return clean(state, errs)
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
			},
			Action: func(c *cli.Context) error {
				state, helm, err := before(c)
				if err != nil {
					return err
				}

				args := c.String("args")
				if len(args) > 0 {
					helm.SetExtraArgs(strings.Split(args, " ")...)
				}

				if c.Bool("sync-repos") {
					if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("err: %s\n", err.Error())
						}
						os.Exit(1)
					}
				}

				values := c.StringSlice("values")

				errs := state.DiffReleases(helm, values)
				return clean(state, errs)
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
				state, helm, err := before(c)
				if err != nil {
					return err
				}

				if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}

				if errs := state.UpdateDeps(helm); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}

				args := c.String("args")
				if len(args) > 0 {
					helm.SetExtraArgs(strings.Split(args, " ")...)
				}

				values := c.StringSlice("values")
				workers := c.Int("concurrency")

				errs := state.SyncReleases(helm, values, workers)
				return clean(state, errs)
			},
		},
		{
			Name:  "delete",
			Usage: "delete charts from state file (helm delete)",
			Action: func(c *cli.Context) error {
				state, helm, err := before(c)
				if err != nil {
					return err
				}

				errs := state.DeleteReleases(helm)
				return clean(state, errs)
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Printf("err: %s", err.Error())
		os.Exit(1)
	}
}

func before(c *cli.Context) (*state.HelmState, helmexec.Interface, error) {
	file := c.GlobalString("file")
	quiet := c.GlobalBool("quiet")
	kubeContext := c.GlobalString("kube-context")
	namespace := c.GlobalString("namespace")
	labels := c.GlobalStringSlice("selector")

	st, err := state.ReadFromFile(file)
	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("open %s:", DefaultHelmfile)) {
			var fallbackErr error
			st, fallbackErr = state.ReadFromFile(DeprecatedHelmfile)
			if fallbackErr != nil {
				return nil, nil, fmt.Errorf("failed to read %s and %s: %v", file, DeprecatedHelmfile, err)
			}
			log.Printf("warn: charts.yaml is loaded: charts.yaml is deprecated in favor of helmfile.yaml. See https://github.com/roboll/helmfile/issues/25 for more information")
		} else {
			return nil, nil, fmt.Errorf("failed to read %s: %v", file, err)
		}
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
