package environment

import (
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/pkg/maputil"
	"gopkg.in/yaml.v2"
)

type Environment struct {
	Name     string
	Values   map[string]interface{}
	Defaults map[string]interface{}
}

var EmptyEnvironment Environment

func (e Environment) DeepCopy() Environment {
	valuesBytes, err := yaml.Marshal(e.Values)
	if err != nil {
		panic(err)
	}
	var values map[string]interface{}
	if err := yaml.Unmarshal(valuesBytes, &values); err != nil {
		panic(err)
	}
	values, err = maputil.CastKeysToStrings(values)
	if err != nil {
		panic(err)
	}

	defaultsBytes, err := yaml.Marshal(e.Defaults)
	if err != nil {
		panic(err)
	}
	var defaults map[string]interface{}
	if err := yaml.Unmarshal(defaultsBytes, &defaults); err != nil {
		panic(err)
	}
	defaults, err = maputil.CastKeysToStrings(defaults)
	if err != nil {
		panic(err)
	}

	return Environment{
		Name:     e.Name,
		Values:   values,
		Defaults: defaults,
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
