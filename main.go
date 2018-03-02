package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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

				if errs := state.SyncRepos(helm); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}
				return nil
			},
		},
		{
			Name:  "charts",
			Usage: "sync charts from state file (helm repo upgrade --install)",
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

				if errs := state.SyncCharts(helm, values, workers); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}
				return nil
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

				if errs := state.DiffCharts(helm, values); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}
				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "sync all resources from state file (repos && charts)",
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

				values := c.StringSlice("values")
				workers := c.Int("concurrency")

				if errs := state.SyncCharts(helm, values, workers); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}
				return nil
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

				if errs := state.DeleteCharts(helm); errs != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s\n", err.Error())
					}
					os.Exit(1)
				}
				return nil
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

	st, err := state.ReadFromFile(file)
	if err != nil && strings.Contains(err.Error(), fmt.Sprintf("open %s:", DefaultHelmfile)) {
		var fallbackErr error
		st, fallbackErr = state.ReadFromFile(DeprecatedHelmfile)
		if fallbackErr != nil {
			return nil, nil, fmt.Errorf("failed to read %s and %s: %v", file, DeprecatedHelmfile, err)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if st.Context != "" {
		if kubeContext != "" {
			log.Printf("err: Cannot use option --kube-context and set attribute context.")
			os.Exit(1)
		}
		kubeContext = st.Context
	}
	if namespace != "" {
		if state.Namespace != "" {
			log.Printf("err: Cannot use option --namespace and set attribute namespace.")
			os.Exit(1)
		}
		state.Namespace = namespace
	}
	var writer io.Writer
	if !quiet {
		writer = os.Stdout
	}

	return st, helmexec.NewHelmExec(writer, kubeContext), nil
}
