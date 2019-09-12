package app

import (
	"fmt"
	"strings"

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

	if errs := r.ctx.SyncReposOnce(r.state, r.helm); errs != nil && len(errs) > 0 {
		return errs
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

func (r *Run) Delete(c DeleteConfigProvider) []error {
	affectedReleases := state.AffectedReleases{}
	purge := c.Purge()

	errs := []error{}

	names := make([]string, len(r.state.Releases))
	for i, r := range r.state.Releases {
		names[i] = fmt.Sprintf("  %s (%s)", r.Name, r.Chart)
	}

	msg := fmt.Sprintf(`Affected releases are:
%s

Do you really want to delete?
  Helmfile will delete all your releases, as shown above.

`, strings.Join(names, "\n"))
	interactive := c.Interactive()
	if !interactive || interactive && r.askForConfirmation(msg) {
		r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

		errs = r.state.DeleteReleases(&affectedReleases, r.helm, c.Concurrency(), purge)
	}
	affectedReleases.DisplayAffectedReleases(c.Logger())
	return errs
}

func (r *Run) Destroy(c DestroyConfigProvider) []error {
	errs := []error{}
	affectedReleases := state.AffectedReleases{}

	names := make([]string, len(r.state.Releases))
	for i, r := range r.state.Releases {
		names[i] = fmt.Sprintf("  %s (%s)", r.Name, r.Chart)
	}

	msg := fmt.Sprintf(`Affected releases are:
%s

Do you really want to delete?
  Helmfile will delete all your releases, as shown above.

`, strings.Join(names, "\n"))
	interactive := c.Interactive()
	if !interactive || interactive && r.askForConfirmation(msg) {
		r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

		errs = r.state.DeleteReleases(&affectedReleases, r.helm, c.Concurrency(), true)
	}
	affectedReleases.DisplayAffectedReleases(c.Logger())
	return errs
}

func (r *Run) Apply(c ApplyConfigProvider) []error {
	st := r.state
	helm := r.helm
	ctx := r.ctx

	affectedReleases := state.AffectedReleases{}
	if !c.SkipDeps() {
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

	// helm must be 2.11+ and helm-diff should be provided `--detailed-exitcode` in order for `helmfile apply` to work properly
	detailedExitCode := true

	diffOpts := &state.DiffOpts{
		NoColor: c.NoColor(),
		Context: c.Context(),
	}

	releases, errs := st.DiffReleases(helm, c.Values(), c.Concurrency(), detailedExitCode, c.SuppressSecrets(), false, diffOpts)

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
			logger := c.Logger()
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
			interactive := c.Interactive()
			if !interactive || interactive && r.askForConfirmation(msg) {
				rs := []state.ReleaseSpec{}
				for _, r := range releases {
					rs = append(rs, *r)
				}
				for _, r := range releasesToBeDeleted {
					rs = append(rs, *r)
				}

				r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

				st.Releases = rs
				return st.SyncReleases(&affectedReleases, helm, c.Values(), c.Concurrency())
			}
		}
	}

	affectedReleases.DisplayAffectedReleases(c.Logger())
	return fatalErrs
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

	_, errs := st.DiffReleases(helm, c.Values(), c.Concurrency(), c.DetailedExitcode(), c.SuppressSecrets(), true)
	return errs
}

func (r *Run) Sync(c SyncConfigProvider) []error {
	st := r.state
	helm := r.helm
	ctx := r.ctx

	affectedReleases := state.AffectedReleases{}
	if !c.SkipDeps() {
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

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	errs := st.SyncReleases(&affectedReleases, helm, c.Values(), c.Concurrency())
	affectedReleases.DisplayAffectedReleases(c.Logger())
	return errs
}

func (r *Run) Template(c TemplateConfigProvider) []error {
	state := r.state
	helm := r.helm
	ctx := r.ctx

	if !c.SkipDeps() {
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

	args := argparser.GetArgs(c.Args(), state)
	return state.TemplateReleases(helm, c.OutputDir(), c.Values(), args, c.Concurrency())
}

func (r *Run) Test(c TestConfigProvider) []error {
	cleanup := c.Cleanup()
	timeout := c.Timeout()
	concurrency := c.Concurrency()

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return r.state.TestReleases(r.helm, cleanup, timeout, concurrency)
}

func (r *Run) Lint(c LintConfigProvider) []error {
	state := r.state
	helm := r.helm
	ctx := r.ctx

	values := c.Values()
	args := argparser.GetArgs(c.Args(), state)
	workers := c.Concurrency()
	if !c.SkipDeps() {
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
}
