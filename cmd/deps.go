package cmd

import (
	"github.com/roboll/helmfile/args"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/pkg/app"
	"github.com/roboll/helmfile/state"
	"github.com/urfave/cli"
)

func Deps(a *app.App) cli.Command {
	return cli.Command{
		Name:  "deps",
		Usage: "update charts based on the contents of requirements.yaml",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "args",
				Value: "",
				Usage: "pass args to helm exec",
			},
		},
		Action: func(c *cli.Context) error {
			return VisitAllDesiredStates(c, func(state *state.HelmState, helm helmexec.Interface, ctx app.Context) (bool, []error) {
				args := args.GetArgs(c.String("args"), state)
				if len(args) > 0 {
					helm.SetExtraArgs(args...)
				}

				errs := state.UpdateDeps(helm)

				ok := len(errs) == 0

				return ok, errs
			})
		},
	}
}
