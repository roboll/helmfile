package maputil

import "fmt"

func CastKeysToStrings(s interface{}) (map[string]interface{}, error) {
	new := map[string]interface{}{}
	switch src := s.(type) {
	case map[interface{}]interface{}:
		for k, v := range src {
			var str_k string
			switch typed_k := k.(type) {
			case string:
				str_k = typed_k
			default:
				return nil, fmt.Errorf("unexpected type of key in map: expected string, got %T: value=%v, map=%v", typed_k, typed_k, src)
			}

			var casted_v interface{}
			switch typed_v := v.(type) {
			case map[interface{}]interface{}:
				tmp, err := CastKeysToStrings(typed_v)
				if err != nil {
					return nil, err
				}
				casted_v = tmp
			default:
				casted_v = typed_v
			}

			new[str_k] = casted_v
		}
	case map[string]interface{}:
		for k, v := range src {
			var casted_v interface{}
			switch typed_v := v.(type) {
			case map[interface{}]interface{}:
				tmp, err := CastKeysToStrings(typed_v)
				if err != nil {
					return nil, err
				}
				casted_v = tmp
			default:
				casted_v = typed_v
			}

			new[k] = casted_v
		}
	}
	return new, nil
}

func Set(m map[string]interface{}, key []string, value string) map[string]interface{} {
	if len(key) == 0 {
		panic(fmt.Errorf("bug: unexpected length of key: %d", len(key)))
	}

	k := key[0]

	if len(key) == 1 {
		m[k] = value
		return m
	}

	remain := key[1:]

	nested, ok := m[k]
	if !ok {
		new_m := map[string]interface{}{}
		nested = Set(new_m, remain, value)
	}

	m[k] = nested

	return m
}
