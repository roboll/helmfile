package cmd

import (
	"fmt"
	"strings"

	"github.com/roboll/helmfile/datasource"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/pkg/app"
	"github.com/roboll/helmfile/state"

	"github.com/urfave/cli"
	"go.uber.org/zap"
)

func VisitAllDesiredStates(c *cli.Context, converge func(*state.HelmState, helmexec.Interface, app.Context) (bool, []error)) error {
	a, fileOrDir, err := InitAppEntry(c, false)
	if err != nil {
		return err
	}

	ctx := app.NewContext()

	convergeWithHelmBinary := func(st *state.HelmState, helm helmexec.Interface) (bool, []error) {
		if c.GlobalString("helm-binary") != "" {
			helm.SetHelmBinary(c.GlobalString("helm-binary"))
		}
		return converge(st, helm, ctx)
	}

	err = a.VisitDesiredStates(fileOrDir, a.Selectors, convergeWithHelmBinary)

	return toCliError(err)
}

func InitAppEntry(c *cli.Context, reverse bool) (*app.App, string, error) {
	datasource.PrepareAll(c)

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

	app := app.Init(&app.App{
		KubeContext: kubeContext,
		Logger:      logger,
		Reverse:     reverse,
		Env:         env,
		Namespace:   namespace,
		Selectors:   selectors,
	})

	return app, fileOrDir, nil
}

func FindAndIterateOverDesiredStatesUsingFlagsWithReverse(c *cli.Context, reverse bool, converge func(*state.HelmState, helmexec.Interface, app.Context) []error) error {
	a, fileOrDir, err := InitAppEntry(c, reverse)
	if err != nil {
		return err
	}

	ctx := app.NewContext()

	convergeWithHelmBinary := func(st *state.HelmState, helm helmexec.Interface) []error {
		if c.GlobalString("helm-binary") != "" {
			helm.SetHelmBinary(c.GlobalString("helm-binary"))
		}
		return converge(st, helm, ctx)
	}

	err = a.VisitDesiredStatesWithReleasesFiltered(fileOrDir, convergeWithHelmBinary)

	return toCliError(err)
}

func toCliError(err error) error {
	if err != nil {
		switch e := err.(type) {
		case *app.NoMatchingHelmfileError:
			return cli.NewExitError(e.Error(), 2)
		case *app.Error:
			return cli.NewExitError(e.Error(), e.Code())
		default:
			panic(fmt.Errorf("BUG: please file an github issue for this unhandled error: %T: %v", e, e))
		}
	}
	return err
}
