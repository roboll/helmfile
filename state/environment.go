package state

type EnvironmentSpec struct {
	Values  []string `yaml:"values"`
	Secrets []string `yaml:"secrets"`
}
