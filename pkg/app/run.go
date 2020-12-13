package app

import (
	"fmt"
	"github.com/roboll/helmfile/pkg/argparser"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/state"
	"io/ioutil"
	"os"
	"sort"
	"strings"
)

type Run struct {
	state *state.HelmState
	helm  helmexec.Interface
	ctx   Context

	ReleaseToChart map[string]string

	Ask func(string) bool
}

func NewRun(st *state.HelmState, helm helmexec.Interface, ctx Context) *Run {
	if helm == nil {
		panic("Assertion failed: helmexec.Interface must not be nil")
	}

	return &Run{state: st, helm: helm, ctx: ctx}
}

func (r *Run) askForConfirmation(msg string) bool {
	if r.Ask != nil {
		return r.Ask(msg)
	}
	return AskForConfirmation(msg)
}

func (r *Run) withPreparedCharts(helmfileCommand string, opts state.ChartPrepareOptions, f func()) error {
	if r.ReleaseToChart != nil {
		panic("Run.PrepareCharts can be called only once")
	}

	if !opts.SkipRepos {
		ctx := r.ctx
		if err := ctx.SyncReposOnce(r.state, r.helm); err != nil {
			return err
		}
	}

	// Create tmp directory and bail immediately if it fails
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	if _, err = r.state.TriggerGlobalPrepareEvent(helmfileCommand); err != nil {
		return err
	}

	releaseToChart, errs := r.state.PrepareCharts(r.helm, dir, 2, helmfileCommand, opts)

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}

	for i := range r.state.Releases {
		rel := &r.state.Releases[i]

		if chart := releaseToChart[rel.Name]; chart != "" {
			rel.Chart = chart
		}
	}

	r.ReleaseToChart = releaseToChart

	f()

	_, err = r.state.TriggerGlobalCleanupEvent(helmfileCommand)

	return err
}

func (r *Run) Deps(c DepsConfigProvider) []error {
	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return r.state.UpdateDeps(r.helm)
}

func (r *Run) Repos(c ReposConfigProvider) error {
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

func (a *App) diff(r *Run, c DiffConfigProvider) (*string, bool, bool, []error) {
	st := r.state

	allReleases := st.GetReleasesWithOverrides()

	toDiff, err := a.getSelectedReleases(r)
	if err != nil {
		return nil, false, false, []error{err}
	}

	if len(toDiff) == 0 {
		return nil, false, false, nil
	}

	// Do build deps and prepare only on selected releases so that we won't waste time
	// on running various helm commands on unnecessary releases
	st.Releases = toDiff

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	opts := &state.DiffOpts{
		Context: c.Context(),
		NoColor: c.NoColor(),
		Set:     c.Set(),
	}

	// Validate all releases for missing `needs` targets
	st.Releases = allReleases

	if _, err := st.PlanReleases(false); err != nil {
		return nil, false, false, []error{err}
	}

	// Diff only targeted releases

	st.Releases = toDiff

	filtered := &Run{
		state: st,
		helm:  r.helm,
		ctx:   r.ctx,
		Ask:   r.Ask,
	}

	infoMsg, updated, deleted, errs := filtered.diff(true, c.DetailedExitcode(), c, opts)

	return infoMsg, true, len(deleted) > 0 || len(updated) > 0, errs
}

func (a *App) test(r *Run, c TestConfigProvider) []error {
	cleanup := c.Cleanup()
	timeout := c.Timeout()
	concurrency := c.Concurrency()

	st := r.state

	toTest, err := a.getSelectedReleases(r)
	if err != nil {
		return []error{err}
	}

	if len(toTest) == 0 {
		return nil
	}

	// Do test only on selected releases, because that's what the user intended
	// with conditions and selectors
	st.Releases = toTest

	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return st.TestReleases(r.helm, cleanup, timeout, concurrency, state.Logs(c.Logs()))
}

func (r *Run) Lint(c LintConfigProvider) []error {
	st := r.state
	helm := r.helm

	values := c.Values()
	args := argparser.GetArgs(c.Args(), st)
	workers := c.Concurrency()
	opts := &state.LintOpts{
		Set: c.Set(),
	}
	return st.LintReleases(helm, values, args, workers, opts)
}

func (run *Run) diff(triggerCleanupEvent bool, detailedExitCode bool, c DiffConfigProvider, diffOpts *state.DiffOpts) (*string, map[string]state.ReleaseSpec, map[string]state.ReleaseSpec, []error) {
	st := run.state
	helm := run.helm

	var changedReleases []state.ReleaseSpec
	var deletingReleases []state.ReleaseSpec
	var planningErrs []error

	// TODO Better way to detect diff on only filtered releases
	{
		changedReleases, planningErrs = st.DiffReleases(helm, c.Values(), c.Concurrency(), detailedExitCode, c.IncludeTests(), c.SuppressSecrets(), c.SuppressDiff(), triggerCleanupEvent, diffOpts)

		var err error
		deletingReleases, err = st.DetectReleasesToBeDeletedForSync(helm, st.Releases)
		if err != nil {
			planningErrs = append(planningErrs, err)
		}
	}

	fatalErrs := []error{}

	for _, e := range planningErrs {
		switch err := e.(type) {
		case *state.ReleaseError:
			if err.Code != 2 {
				fatalErrs = append(fatalErrs, e)
			}
		default:
			fatalErrs = append(fatalErrs, e)
		}
	}

	if len(fatalErrs) > 0 {
		return nil, nil, nil, fatalErrs
	}

	releasesToBeDeleted := map[string]state.ReleaseSpec{}
	for _, r := range deletingReleases {
		id := state.ReleaseToID(&r)
		releasesToBeDeleted[id] = r
	}

	releasesToBeUpdated := map[string]state.ReleaseSpec{}
	for _, r := range changedReleases {
		id := state.ReleaseToID(&r)

		// If `helm-diff` detected changes but it is not being `helm delete`ed, we should run `helm upgrade`
		if _, ok := releasesToBeDeleted[id]; !ok {
			releasesToBeUpdated[id] = r
		}
	}

	// sync only when there are changes
	if len(releasesToBeUpdated) == 0 && len(releasesToBeDeleted) == 0 {
		var msg *string
		if c.DetailedExitcode() {
			// TODO better way to get the logger
			m := "No affected releases"
			msg = &m
		}
		return msg, nil, nil, nil
	}

	names := []string{}
	for _, r := range releasesToBeUpdated {
		names = append(names, fmt.Sprintf("  %s (%s) UPDATED", r.Name, r.Chart))
	}
	for _, r := range releasesToBeDeleted {
		names = append(names, fmt.Sprintf("  %s (%s) DELETED", r.Name, r.Chart))
	}
	// Make the output deterministic for testing purpose
	sort.Strings(names)

	infoMsg := fmt.Sprintf(`Affected releases are:
%s
`, strings.Join(names, "\n"))

	return &infoMsg, releasesToBeUpdated, releasesToBeDeleted, nil
}
