package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/roboll/helmfile/pkg/argparser"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/state"
)

type Run struct {
	state *state.HelmState
	helm  helmexec.Interface
	ctx   Context

	ReleaseToChart map[state.PrepareChartKey]string

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
	var dir string
	if len(opts.OutputDir) == 0 {
		tempDir, err := ioutil.TempDir("", "helmfile*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		dir = tempDir
	} else {
		dir = opts.OutputDir
		fmt.Printf("Charts will be downloaded to: %s\n", dir)
	}

	if _, err := r.state.TriggerGlobalPrepareEvent(helmfileCommand); err != nil {
		return err
	}

	releaseToChart, errs := r.state.PrepareCharts(r.helm, dir, 2, helmfileCommand, opts)

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}

	for i := range r.state.Releases {
		rel := &r.state.Releases[i]

		key := state.PrepareChartKey{
			Name:        rel.Name,
			Namespace:   rel.Namespace,
			KubeContext: rel.KubeContext,
		}
		if chart := releaseToChart[key]; chart != "" {
			rel.Chart = chart
		}
	}

	r.ReleaseToChart = releaseToChart

	f()

	_, err := r.state.TriggerGlobalCleanupEvent(helmfileCommand)

	return err
}

func (r *Run) Deps(c DepsConfigProvider) []error {
	r.helm.SetExtraArgs(argparser.GetArgs(c.Args(), r.state)...)

	return r.state.UpdateDeps(r.helm, c.IncludeTransitiveNeeds())
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

func (r *Run) diff(triggerCleanupEvent bool, detailedExitCode bool, c DiffConfigProvider, diffOpts *state.DiffOpts) (*string, map[string]state.ReleaseSpec, map[string]state.ReleaseSpec, []error) {
	st := r.state
	helm := r.helm

	var changedReleases []state.ReleaseSpec
	var deletingReleases []state.ReleaseSpec
	var planningErrs []error

	// TODO Better way to detect diff on only filtered releases
	{
		changedReleases, planningErrs = st.DiffReleases(helm, c.Values(), c.Concurrency(), detailedExitCode, c.IncludeTests(), c.Suppress(), c.SuppressSecrets(), c.ShowSecrets(), c.SuppressDiff(), triggerCleanupEvent, diffOpts)

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
		release := r
		id := state.ReleaseToID(&release)
		releasesToBeDeleted[id] = release
	}

	releasesToBeUpdated := map[string]state.ReleaseSpec{}
	for _, r := range changedReleases {
		release := r
		id := state.ReleaseToID(&release)

		// If `helm-diff` detected changes but it is not being `helm delete`ed, we should run `helm upgrade`
		if _, ok := releasesToBeDeleted[id]; !ok {
			releasesToBeUpdated[id] = release
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
