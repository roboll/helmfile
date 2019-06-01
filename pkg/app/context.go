package app

import (
	"github.com/roboll/helmfile/pkg/state"
)

type Context struct {
	updatedRepos map[string]struct{}
}

func NewContext() Context {
	return Context{
		updatedRepos: map[string]struct{}{},
	}
}

func (ctx Context) SyncReposOnce(st *state.HelmState, helm state.RepoUpdater) []error {
	var errs []error

	allUpdated := true
	for _, r := range st.Repositories {
		_, exists := ctx.updatedRepos[r.Name]
		allUpdated = allUpdated && exists
	}

	if !allUpdated {
		errs = st.SyncRepos(helm)

		for _, r := range st.Repositories {
			ctx.updatedRepos[r.Name] = struct{}{}
		}
	}

	return errs
}
