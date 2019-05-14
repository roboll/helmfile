package app

import (
	"os"
	"strings"
)

const (
	DefaultHelmfile              = "helmfile.yaml"
	DeprecatedHelmfile           = "charts.yaml"
	DefaultHelmfileDirectory     = "helmfile.d"
	ExperimentalEnvVar           = "HELMFILE_EXPERIMENTAL"         // environment variable for experimental features, expecting "true" lower case
	ExperimentalSelectorExplicit = "explicit-selector-inheritance" // value to remove default selector inheritance to sub-helmfiles and use the explicit one
)

func experimentalModeEnabled() bool {
	return os.Getenv(ExperimentalEnvVar) == "true"
}

func isExplicitSelectorInheritanceEnabled() bool {
	return experimentalModeEnabled() || strings.Contains(os.Getenv(ExperimentalEnvVar), ExperimentalSelectorExplicit)
}
