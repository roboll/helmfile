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

func get(path string, o interface{}) (interface{}, error) {
	if path == "" {
		return o, nil
	}
	keys := strings.Split(path, ".")
	switch oo := o.(type) {
	case map[string]interface{}:
		v, ok := oo[keys[0]]
		if !ok {
			return nil, &noValueError{fmt.Sprintf("no value exist for key \"%s\" in %v", keys[0], oo)}
		}
		return get(strings.Join(keys[1:], "."), v)
	case map[interface{}]interface{}:
		v, ok := oo[keys[0]]
		if !ok {
			return nil, &noValueError{fmt.Sprintf("no value exist for key \"%s\" in %v", keys[0], oo)}
		}
		return get(strings.Join(keys[1:], "."), v)
	default:
		maybeStruct := reflect.ValueOf(oo).Elem()
		if maybeStruct.NumField() < 1 {
			return nil, &noValueError{fmt.Sprintf("unexpected type(%v) of value for key \"%s\": it must be either map[string]interface{} or any struct", reflect.TypeOf(o), keys[0])}
		}
		f := maybeStruct.FieldByName(keys[0])
		if !f.IsValid() {
			return nil, &noValueError{fmt.Sprintf("no field named \"%s\" exist in %v", keys[0], oo)}
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
