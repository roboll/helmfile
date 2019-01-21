package state

import "github.com/roboll/helmfile/environment"

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
}

// ReleaseTemplateData provides variables accessible while executing golang text/template expressions in releases of a helmfile YAML file
type ReleaseTemplateData struct {
	// Environment is accessible as `.Environment` from any template expression executed by the renderer
	Environment environment.Environment
	// Release is accessible as `.Release` from any template expression executed by the renderer
	Release ReleaseSpec
}
