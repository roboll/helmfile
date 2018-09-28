package tmpl

import (
	"fmt"
	"reflect"
	"strings"
)

type noValueError struct {
	msg string
}

func (e *noValueError) Error() string {
	return e.msg
}

func get(path string, obj interface{}) (interface{}, error) {
	if path == "" {
		return obj, nil
	}
	keys := strings.Split(path, ".")
	switch typedObj := obj.(type) {
	case map[string]interface{}:
		v, ok := typedObj[keys[0]]
		if !ok {
			return nil, &noValueError{fmt.Sprintf("no value exist for key \"%s\" in %v", keys[0], typedObj)}
		}
		return get(strings.Join(keys[1:], "."), v)
	case map[interface{}]interface{}:
		v, ok := typedObj[keys[0]]
		if !ok {
			return nil, &noValueError{fmt.Sprintf("no value exist for key \"%s\" in %v", keys[0], typedObj)}
		}
		return get(strings.Join(keys[1:], "."), v)
	default:
		maybeStruct := reflect.ValueOf(typedObj)
		if maybeStruct.NumField() < 1 {
			return nil, &noValueError{fmt.Sprintf("unexpected type(%v) of value for key \"%s\": it must be either map[string]interface{} or any struct", reflect.TypeOf(obj), keys[0])}
		}
		f := maybeStruct.FieldByName(keys[0])
		if !f.IsValid() {
			return nil, &noValueError{fmt.Sprintf("no field named \"%s\" exist in %v", keys[0], typedObj)}
		}
		v := f.Interface()
		return get(strings.Join(keys[1:], "."), v)
	}
}

func getOrNil(path string, o interface{}) (interface{}, error) {
	v, err := get(path, o)
	if err != nil {
		switch err.(type) {
		case *noValueError:
			return nil, nil
		default:
			return nil, err
		}
	}
	return v, nil
}
