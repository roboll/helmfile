package helmexec

type Interface interface {
	SetExtraArgs(args ...string)

	AddRepo(name, repository string) error
	UpdateRepo() error

	SyncChart(name, chart string, flags ...string) error
	DeleteChart(name string) error
}
