package environment

import "encoding/json"

type Environment struct {
	Name   string
	Values map[string]interface{}
}

var EmptyEnvironment Environment

func (e Environment) DeepCopy() Environment {
	bytes, err := json.Marshal(e.Values)
	if err != nil {
		panic(err)
	}
	var values map[string]interface{}
	if err := json.Unmarshal(bytes, &values); err != nil {
		panic(err)
	}
	return Environment{
		Name:   e.Name,
		Values: values,
	}
}
