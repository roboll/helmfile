package environment

import (
	"github.com/imdario/mergo"
	"gopkg.in/yaml.v2"
)

type Environment struct {
	Name   string
	Values map[string]interface{}
}

var EmptyEnvironment Environment

func (e Environment) DeepCopy() Environment {
	bytes, err := yaml.Marshal(e.Values)
	if err != nil {
		panic(err)
	}
	var values map[string]interface{}
	if err := yaml.Unmarshal(bytes, &values); err != nil {
		panic(err)
	}
	return Environment{
		Name:   e.Name,
		Values: values,
	}
}

func (e *Environment) Merge(other *Environment) (*Environment, error) {
	if e == nil {
		if other != nil {
			copy := other.DeepCopy()
			return &copy, nil
		}
		return nil, nil
	}
	copy := e.DeepCopy()
	if other != nil {
		if err := mergo.Merge(&copy, other, mergo.WithOverride); err != nil {
			return nil, err
		}
	}
	return &copy, nil
}
