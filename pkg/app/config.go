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

type ApplyConfigProvider interface {
	Values() []string
	SkipDeps() bool

	SuppressSecrets() bool

	concurrencyConfig
	interactive
	loggingConfig
}

type SyncConfigProvider interface {
	Values() []string
	SkipDeps() bool

	concurrencyConfig
	loggingConfig
}

type DiffConfigProvider interface {
	Values() []string
	SkipDeps() bool

	SuppressSecrets() bool

	DetailedExitcode() bool

	concurrencyConfig
}

type DeleteConfigProvider interface {
	Purge() bool

	interactive
	loggingConfig
}

type DestroyConfigProvider interface {
	interactive
	loggingConfig
}

type TestConfigProvider interface {
	Timeout() int
	Cleanup() bool

	concurrencyConfig
}

type LintConfigProvider interface {
	Values() []string
	SkipDeps() bool

	Args() string

	concurrencyConfig
}

type TemplateConfigProvider interface {
	Values() []string
	SkipDeps() bool

	Args() string

	concurrencyConfig
}

type StatusesConfigProvider interface {
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
