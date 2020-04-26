package app

import "github.com/roboll/helmfile/pkg/helmexec"

type noCallHelmExec struct {
}

func (helm *noCallHelmExec) doPanic() {
	panic("unexpected call to helm")
}

func (helm *noCallHelmExec) TemplateRelease(name, chart string, flags ...string) error {
	helm.doPanic()
	return nil
}

func (helm *noCallHelmExec) UpdateDeps(chart string) error {
	helm.doPanic()
	return nil
}

func (helm *noCallHelmExec) BuildDeps(name, chart string) error {
	helm.doPanic()
	return nil
}

func (helm *noCallHelmExec) SetExtraArgs(args ...string) {
	helm.doPanic()
	return
}
func (helm *noCallHelmExec) SetHelmBinary(bin string) {
	helm.doPanic()
	return
}
func (helm *noCallHelmExec) AddRepo(name, repository, cafile, certfile, keyfile, username, password string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) UpdateRepo() error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) SyncRelease(context helmexec.HelmContext, name, chart string, flags ...string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) DiffRelease(context helmexec.HelmContext, name, chart string, suppressDiff bool, flags ...string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) ReleaseStatus(context helmexec.HelmContext, release string, flags ...string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) DeleteRelease(context helmexec.HelmContext, name string, flags ...string) error {
	helm.doPanic()
	return nil
}

func (helm *noCallHelmExec) List(context helmexec.HelmContext, filter string, flags ...string) (string, error) {
	helm.doPanic()
	return "", nil
}

func (helm *noCallHelmExec) DecryptSecret(context helmexec.HelmContext, name string, flags ...string) (string, error) {
	helm.doPanic()
	return "", nil
}
func (helm *noCallHelmExec) TestRelease(context helmexec.HelmContext, name string, flags ...string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) Fetch(chart string, flags ...string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) Lint(name, chart string, flags ...string) error {
	helm.doPanic()
	return nil
}
func (helm *noCallHelmExec) IsHelm3() bool {
	helm.doPanic()
	return false
}

func (helm *noCallHelmExec) GetVersion() helmexec.Version {
	helm.doPanic()
	return helmexec.Version{}
}

func (helm *noCallHelmExec) IsVersionAtLeast(major int, minor int) bool {
	helm.doPanic()
	return false
}
