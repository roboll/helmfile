package helmexec

// Interface for executing helm commands
type Interface interface {
	SetExtraArgs(args ...string)
	SetHelmBinary(bin string)

	AddRepo(name, repository, certfile, keyfile, username, password string) error
	UpdateRepo() error
	BuildDeps(chart string) error
	UpdateDeps(chart string) error
	SyncRelease(name, chart string, flags ...string) error
	DiffRelease(name, chart string, flags ...string) error
	TemplateRelease(chart string, flags ...string) error
	Fetch(chart string, flags ...string) error
	Lint(chart string, flags ...string) error
	ReleaseStatus(name string) error
	DeleteRelease(name string, flags ...string) error
	TestRelease(name string, flags ...string) error
	List(filter string) (string, error)
	DecryptSecret(name string) (string, error)
}
