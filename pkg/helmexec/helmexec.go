package helmexec

// Interface for executing helm commands
type Interface interface {
	SetExtraArgs(args ...string)
	SetHelmBinary(bin string)

	AddRepo(name, repository, cafile, certfile, keyfile, username, password string) error
	UpdateRepo() error
	BuildDeps(name, chart string) error
	UpdateDeps(chart string) error
	SyncRelease(context HelmContext, name, chart string, flags ...string) error
	DiffRelease(context HelmContext, name, chart string, flags ...string) error
	TemplateRelease(name, chart string, flags ...string) error
	Fetch(chart string, flags ...string) error
	Lint(name, chart string, flags ...string) error
	ReleaseStatus(context HelmContext, name string, flags ...string) error
	DeleteRelease(context HelmContext, name string, flags ...string) error
	TestRelease(context HelmContext, name string, flags ...string) error
	List(context HelmContext, filter string, flags ...string) (string, error)
	DecryptSecret(context HelmContext, name string, flags ...string) (string, error)
	IsHelm3() bool
}

type DependencyUpdater interface {
	UpdateDeps(chart string) error
	IsHelm3() bool
}
