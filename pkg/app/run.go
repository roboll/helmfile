package app

import (
	"github.com/roboll/helmfile/pkg/argparser"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/state"
)

type Run struct {
	state *state.HelmState
	helm  helmexec.Interface
	ctx   Context

	Ask func(string) bool
}

func NewRun(st *state.HelmState, helm helmexec.Interface, ctx Context) *Run {
	return &Run{state: st, helm: helm, ctx: ctx}
}

func (r *Run) askForConfirmation(msg string) bool {
	if r.Ask != nil {
		return r.Ask(msg)
	}
	return AskForConfirmation(msg)
}

func (r *Run) Deps(c DepsConfigProvider) []error {
	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	if !c.SkipRepos() {
		if errs := r.ctx.SyncReposOnce(r.state, r.helm); errs != nil && len(errs) > 0 {
			return errs
		}
	}

	return r.state.UpdateDeps(r.helm)
}

func (r *Run) Repos(c ReposConfigProvider) []error {
	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return r.ctx.SyncReposOnce(r.state, r.helm)
}

func (r *Run) DeprecatedSyncCharts(c DeprecatedChartsConfigProvider) []error {
	st := r.state
	helm := r.helm

	affectedReleases := state.AffectedReleases{}
	errs := st.SyncReleases(&affectedReleases, helm, c.Values(), c.Concurrency())
	affectedReleases.DisplayAffectedReleases(c.Logger())
	return errs
}

func (r *Run) Status(c StatusesConfigProvider) []error {
	workers := c.Concurrency()

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return r.state.ReleaseStatuses(r.helm, workers)
}

func (r *Run) Diff(c DiffConfigProvider) []error {
	st := r.state
	helm := r.helm
	ctx := r.ctx

	if !c.SkipDeps() {
		if errs := ctx.SyncReposOnce(st, helm); errs != nil && len(errs) > 0 {
			return errs
		}
		if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
			return errs
		}
	}
	if errs := st.PrepareReleases(helm, "diff"); errs != nil && len(errs) > 0 {
		return errs
	}

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	opts := &state.DiffOpts{
		Context: c.Context(),
		NoColor: c.NoColor(),
		Set:     c.Set(),
	}
	_, errs := st.DiffReleases(helm, c.Values(), c.Concurrency(), c.DetailedExitcode(), c.SuppressSecrets(), true, opts)
	return errs
}

func (r *Run) Test(c TestConfigProvider) []error {
	cleanup := c.Cleanup()
	timeout := c.Timeout()
	concurrency := c.Concurrency()

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return r.state.TestReleases(r.helm, cleanup, timeout, concurrency)
}

func (r *Run) Lint(c LintConfigProvider) []error {
	st := r.state
	helm := r.helm
	ctx := r.ctx

	values := c.Values()
	args := argparser.GetArgs(c.Args(), st)
	workers := c.Concurrency()
	if !c.SkipDeps() {
		if errs := ctx.SyncReposOnce(st, helm); errs != nil && len(errs) > 0 {
			return errs
		}
		if errs := st.BuildDeps(helm); errs != nil && len(errs) > 0 {
			return errs
		}
	}
	if errs := st.PrepareReleases(helm, "lint"); errs != nil && len(errs) > 0 {
		return errs
	}
	opts := &state.LintOpts{
		Set: c.Set(),
	}
	return st.LintReleases(helm, values, args, workers, opts)
}
