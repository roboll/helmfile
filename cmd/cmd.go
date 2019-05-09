package cmd

import (
	"fmt"
	"github.com/roboll/helmfile/helmexec"
	"github.com/roboll/helmfile/pkg/app"
	"github.com/roboll/helmfile/state"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"os/exec"
	"strings"
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
		case *exec.ExitError:
			panic(fmt.Sprintf("BUG: there should be no unhandled *exec.ExitError!: %v", e))
			//// Propagate any non-zero exit status from the external command like `helm` that is failed under the hood
			//status := e.Sys().(syscall.WaitStatus)
			//return cli.NewExitError(e.Error(), status.ExitStatus())
		case *state.ReleaseError:
			panic(fmt.Sprintf("BUG: there should be no unhandled *state.ReleaseError!: %v", e))
		case *app.Error:
			return cli.NewExitError(e.Error(), e.Code())
		default:
			panic(fmt.Errorf("unexpected error: %T: %v", e, e))
			//return cli.NewExitError(e.Error(), 1)
		}
	}
	return err
}
