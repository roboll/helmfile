package state

import (
	"github.com/roboll/helmfile/pkg/environment"
)

// TemplateSpec defines the structure of a reusable and composable template for helm releases.
type TemplateSpec struct {
	ReleaseSpec `yaml:",inline"`
}

// EnvironmentTemplateData provides variables accessible while executing golang text/template expressions in helmfile and values YAML files
type EnvironmentTemplateData struct {
	// Environment is accessible as `.Environment` from any template executed by the renderer
	Environment environment.Environment
	// Namespace is accessible as `.Namespace` from any non-values template executed by the renderer
	Namespace string
	// Values is accessible as `.Values` and it contains default state values overrode by environment values and override values.
	Values map[string]interface{}
}

// releaseTemplateData provides variables accessible while executing golang text/template expressions in release templates
// and release values templates within a Helmfile YAML file.
type releaseTemplateData struct {
	// Environment is accessible as `.Environment` from any template expression executed by the renderer
	Environment environment.Environment
	// Release is accessible as `.Release` from any template expression executed by the renderer.
	// It contains a subset of ReleaseSpec that is known to be useful to dynamically render values.
	Release releaseTemplateDataRelease
	// Values is accessible as `.Values` and it contains default state values overrode by environment values and override values.
	Values map[string]interface{}
	// Namespace is HelmState.OverrideNamespace.
	// You should better use Release.Namespace as it might work as you'd expect even if OverrideNamespace is not set.
	// See releaseTemplateDataRelease.Namespace for more information.
	Namespace string
}

type releaseTemplateDataRelease struct {
	// Name is basically ReleaseSpec.Name exposed to the template
	Name string

	// Namespace is HelmState.OverrideNamespace, or if it's empty, ReleaseSpec.Namespace.
	Namespace string

	// Labels is ReleaseSpec.Labels
	Labels map[string]string

	// Chart is ReleaseSpec.Chart
	Chart string
}
