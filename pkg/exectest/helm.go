package exectest

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/roboll/helmfile/pkg/helmexec"
)

type ListKey struct {
	Filter string
	Flags  string
}

type DiffKey struct {
	Name  string
	Chart string
	Flags string
}

type Helm struct {
	Charts               []string
	Repo                 []string
	Releases             []Release
	Deleted              []Release
	Lists                map[ListKey]string
	Diffs                map[DiffKey]error
	Diffed               []Release
	FailOnUnexpectedDiff bool
	FailOnUnexpectedList bool

	UpdateDepsCallbacks map[string]func(string) error

	DiffMutex     *sync.Mutex
	ChartsMutex   *sync.Mutex
	ReleasesMutex *sync.Mutex
}

type Release struct {
	Name  string
	Flags []string
}

type Affected struct {
	Upgraded []*Release
	Deleted  []*Release
	Failed   []*Release
}

func (helm *Helm) UpdateDeps(chart string) error {
	if strings.Contains(chart, "error") {
		return fmt.Errorf("simulated UpdateDeps failure for chart: %s", chart)
	}
	helm.Charts = append(helm.Charts, chart)

	if helm.UpdateDepsCallbacks != nil {
		callback, exists := helm.UpdateDepsCallbacks[chart]
		if exists {
			if err := callback(chart); err != nil {
				return err
			}
		}
	}
	return nil
}

func (helm *Helm) BuildDeps(name, chart string) error {
	if strings.Contains(chart, "error") {
		return errors.New("error")
	}
	helm.Charts = append(helm.Charts, chart)
	return nil
}

func (helm *Helm) SetExtraArgs(args ...string) {
	return
}
func (helm *Helm) SetHelmBinary(bin string) {
	return
}
func (helm *Helm) AddRepo(name, repository, cafile, certfile, keyfile, username, password string) error {
	helm.Repo = []string{name, repository, cafile, certfile, keyfile, username, password}
	return nil
}
func (helm *Helm) UpdateRepo() error {
	return nil
}
func (helm *Helm) SyncRelease(context helmexec.HelmContext, name, chart string, flags ...string) error {
	if strings.Contains(name, "error") {
		return errors.New("error")
	}
	helm.sync(helm.ReleasesMutex, func() {
		helm.Releases = append(helm.Releases, Release{Name: name, Flags: flags})
	})
	helm.sync(helm.ChartsMutex, func() {
		helm.Charts = append(helm.Charts, chart)
	})

	return nil
}
func (helm *Helm) DiffRelease(context helmexec.HelmContext, name, chart string, suppressDiff bool, flags ...string) error {
	if helm.DiffMutex != nil {
		helm.DiffMutex.Lock()
	}
	helm.Diffed = append(helm.Diffed, Release{Name: name, Flags: flags})
	if helm.DiffMutex != nil {
		helm.DiffMutex.Unlock()
	}
	key := DiffKey{Name: name, Chart: chart, Flags: strings.Join(flags, "")}
	err, ok := helm.Diffs[key]
	if !ok && helm.FailOnUnexpectedDiff {
		return fmt.Errorf("unexpected diff with key: %v", key)
	}
	return err
}
func (helm *Helm) ReleaseStatus(context helmexec.HelmContext, release string, flags ...string) error {
	if strings.Contains(release, "error") {
		return errors.New("error")
	}
	helm.Releases = append(helm.Releases, Release{Name: release, Flags: flags})
	return nil
}
func (helm *Helm) DeleteRelease(context helmexec.HelmContext, name string, flags ...string) error {
	if strings.Contains(name, "error") {
		return errors.New("error")
	}
	helm.Deleted = append(helm.Deleted, Release{Name: name, Flags: flags})
	return nil
}
func (helm *Helm) List(context helmexec.HelmContext, filter string, flags ...string) (string, error) {
	key := ListKey{Filter: filter, Flags: strings.Join(flags, "")}
	res, ok := helm.Lists[key]
	if !ok && helm.FailOnUnexpectedList {
		return "", fmt.Errorf("unexpected list key: %v", key)
	}
	return res, nil
}
func (helm *Helm) DecryptSecret(context helmexec.HelmContext, name string, flags ...string) (string, error) {
	return "", nil
}
func (helm *Helm) TestRelease(context helmexec.HelmContext, name string, flags ...string) error {
	if strings.Contains(name, "error") {
		return errors.New("error")
	}
	helm.Releases = append(helm.Releases, Release{Name: name, Flags: flags})
	return nil
}
func (helm *Helm) Fetch(chart string, flags ...string) error {
	return nil
}
func (helm *Helm) Lint(name, chart string, flags ...string) error {
	return nil
}
func (helm *Helm) TemplateRelease(name, chart string, flags ...string) error {
	return nil
}

func (helm *Helm) IsHelm3() bool {
	return false
}

func (helm *Helm) sync(m *sync.Mutex, f func()) {
	if m != nil {
		m.Lock()
		defer m.Unlock()
	}

	f()
}
