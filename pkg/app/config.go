package app

import "go.uber.org/zap"

type ConfigProvider interface {
	Args() string
	HelmBinary() string

	FileOrDir() string
	KubeContext() string
	Namespace() string
	Selectors() []string
	Env() string

	loggingConfig
}

type DeprecatedChartsConfigProvider interface {
	Values() []string

	concurrencyConfig
	loggingConfig
}

type DepsConfigProvider interface {
	Args() string
}

type ReposConfigProvider interface {
	Args() string
}

type ApplyConfigProvider interface {
	Args() string

	Values() []string
	SkipDeps() bool

	SuppressSecrets() bool

	concurrencyConfig
	interactive
	loggingConfig
}

type SyncConfigProvider interface {
	Args() string

	Values() []string
	SkipDeps() bool

	concurrencyConfig
	loggingConfig
}

type DiffConfigProvider interface {
	Args() string

	Values() []string
	SkipDeps() bool

	SuppressSecrets() bool

	DetailedExitcode() bool

	concurrencyConfig
}

type DeleteConfigProvider interface {
	Args() string

	Purge() bool

	interactive
	loggingConfig
}

type DestroyConfigProvider interface {
	Args() string

	interactive
	loggingConfig
}

type TestConfigProvider interface {
	Args() string

	Timeout() int
	Cleanup() bool

	concurrencyConfig
}

type LintConfigProvider interface {
	Args() string

	Values() []string
	SkipDeps() bool

	concurrencyConfig
}

type TemplateConfigProvider interface {
	Args() string

	Values() []string
	SkipDeps() bool

	concurrencyConfig
}

type StatusesConfigProvider interface {
	Args() string

	concurrencyConfig
}

type concurrencyConfig interface {
	Concurrency() int
}

type loggingConfig interface {
	Logger() *zap.SugaredLogger
}

type interactive interface {
	Interactive() bool
}
