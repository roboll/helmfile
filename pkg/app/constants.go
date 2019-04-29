package app

import (
	"os"
	"strings"
)

const (
	DefaultHelmfile              = "helmfile.yaml"
	DeprecatedHelmfile           = "charts.yaml"
	DefaultHelmfileDirectory     = "helmfile.d"
	ExperimentalEnvVar           = "EXPERIMENTAL"                  // environment variable for experimental features, expecting "true" lower case
	ExperimentalSelectorExplicit = "explicit-selector-inheritance" // value to remove default selector inheritance to sub-helmfiles and use the explicit one
)

func isExplicitSelectorInheritanceEnabled() bool {
	return os.Getenv(ExperimentalEnvVar) == "true" || strings.Contains(os.Getenv(ExperimentalEnvVar), ExperimentalSelectorExplicit)
}
