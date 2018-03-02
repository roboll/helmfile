package helmexec

type Interface interface {
	SetExtraArgs(args ...string)

	AddRepo(name, repository string) error
	UpdateRepo() error

	SyncRelease(name, chart string, flags ...string) error
	DiffRelease(name, chart string, flags ...string) error
	DeleteRelease(name string) error
}
