package helmexec

import "io"

type Interface interface {
	SetExtraArgs(args ...string)

	AddRepo(name, repository, certfile, keyfile string) error
	UpdateRepo() error

	SyncRelease(name, chart string, flags ...string) error
	DiffRelease(name, chart string, flags ...string) error
	DeleteRelease(name string) error

	DecryptSecret(name string) (string, error)

	// unit testing
	exec(args ...string) ([]byte, error)
	setRunner(runner Runner)
	getExtra() []string
	getKubeContent() string
	getWriter() io.Writer
}
