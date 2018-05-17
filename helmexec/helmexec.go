package helmexec

// Interface for executing helm commands
type Interface interface {
	SetExtraArgs(args ...string)

	AddRepo(name, repository, certfile, keyfile string) error
	UpdateRepo() error
	UpdateDeps(chart string) error
	SyncRelease(name, chart string, flags ...string) error
	DiffRelease(name, chart string, flags ...string) error
	ReleaseStatus(name string) error
	DeleteRelease(name string, flags ...string) error
	TestRelease(name string, flags ...string) error

	DecryptSecret(name string) (string, error)
}

