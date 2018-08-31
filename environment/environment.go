package environment

type Environment struct {
	Name   string
	Values map[string]interface{}
}

var EmptyEnvironment Environment
