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
	helmfile = "charts.yaml"
)

func main() {

	app := cli.NewApp()
	app.Name = "helmfile"
	app.Usage = ""
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "file, f",
			Value: helmfile,
			Usage: "load config from `FILE`",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "silence output",
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

				if errs := state.SyncRepos(helm); err != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s", err.Error())
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

				if errs := state.SyncCharts(helm); err != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s", err.Error())
					}
					os.Exit(1)
				}
				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "sync all resources from state file (repos && charts)",
			Action: func(c *cli.Context) error {
				state, helm, err := before(c)
				if err != nil {
					return err
				}

				if errs := state.SyncRepos(helm); err != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s", err.Error())
					}
					os.Exit(1)
				}

				if errs := state.SyncCharts(helm); err != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s", err.Error())
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

				if errs := state.DeleteCharts(helm); err != nil && len(errs) > 0 {
					for _, err := range errs {
						fmt.Printf("err: %s", err.Error())
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

	state, err := state.ReadFromFile(file)
	if err != nil {
		return nil, nil, err
	}

	var writer io.Writer
	if !quiet {
		writer = os.Stdout
	}

	return state, helmexec.NewHelmExec(writer), nil
}
