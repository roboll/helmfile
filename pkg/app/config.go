package app

import "go.uber.org/zap"

type ConfigProvider interface {
	Args() string
	HelmBinary() string

	FileOrDir() string
	KubeContext() string
	Namespace() string
	Selectors() []string
	StateValuesSet() map[string]interface{}
	StateValuesFiles() []string
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
	SkipRepos() bool
}

type ReposConfigProvider interface {
	Args() string
}

type ApplyConfigProvider interface {
	Args() string

	Values() []string
	Set() []string
	SkipDeps() bool

	IncludeTests() bool

	SuppressSecrets() bool
	SuppressDiff() bool

	DetailedExitcode() bool

	NoColor() bool
	Context() int

	RetainValuesFiles() bool
	SkipCleanup() bool
	SkipDiffOnInstall() bool

	concurrencyConfig
	interactive
	loggingConfig
}

type SyncConfigProvider interface {
	Args() string

	Values() []string
	Set() []string
	SkipDeps() bool

	concurrencyConfig
	loggingConfig
}

type DiffConfigProvider interface {
	Args() string

	Values() []string
	Set() []string
	SkipDeps() bool

	IncludeTests() bool

	SuppressSecrets() bool
	SuppressDiff() bool

	DetailedExitcode() bool
	NoColor() bool
	Context() int

	concurrencyConfig
}

type DeleteConfigProvider interface {
	Args() string

	Purge() bool

	interactive
	loggingConfig
	concurrencyConfig
}

type DestroyConfigProvider interface {
	Args() string

	interactive
	loggingConfig
	concurrencyConfig
}

type TestConfigProvider interface {
	Args() string

	Timeout() int
	Cleanup() bool
	Logs() bool

	concurrencyConfig
}

type LintConfigProvider interface {
	Args() string

	Values() []string
	Set() []string
	SkipDeps() bool

	concurrencyConfig
}

type TemplateConfigProvider interface {
	Args() string

	Values() []string
	Set() []string
	OutputDirTemplate() string
	Validate() bool
	SkipDeps() bool
	SkipCleanup() bool
	OutputDir() string
	IncludeCRDs() bool

	concurrencyConfig
}

type WriteValuesConfigProvider interface {
	Values() []string
	Set() []string
	OutputFileTemplate() string
	SkipDeps() bool
}

type StatusesConfigProvider interface {
	Args() string

	concurrencyConfig
}

type StateConfigProvider interface {
	EmbedValues() bool
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

type ListConfigProvider interface {
	Output() string
}
