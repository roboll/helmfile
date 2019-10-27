package vals

import (
	"fmt"
	"github.com/variantdev/vals/pkg/api"
)

type mapConfig struct {
	m map[string]interface{}
}

func (m mapConfig) String(path ...string) string {
	var cur interface{}

	cur = m.m

	for i, k := range path {
		switch typed := cur.(type) {
		case map[string]interface{}:
			cur = typed[k]
		case map[interface{}]interface{}:
			cur = typed[k]
		default:
			return ""
		}
		if i == len(path)-1 {
			if cur == nil {
				return ""
			}
			return fmt.Sprintf("%v", cur)
		}
	}

	panic("invalid state")
}

func (m mapConfig) StringSlice(path ...string) []string {
	var cur interface{}

	cur = m.m

	for i, k := range path {
		switch typed := cur.(type) {
		case map[string]interface{}:
			cur = typed[k]
		case map[interface{}]interface{}:
			cur = typed[k]
		default:
			return nil
		}
		if i == len(path)-1 {
			if cur == nil {
				return nil
			}
			switch ary := cur.(type) {
			case []string:
				return ary
			case []interface{}:
				ss := make([]string, len(ary))
				for i := range ary {
					ss[i] = fmt.Sprintf("%v", ary[i])
				}
				return ss
			default:
				panic(fmt.Errorf("unexpected type: value=%v, type=%T", ary, ary))
			}
		}
	}

	panic("invalid state")
}

func setValue(m map[string]interface{}, v interface{}, path ...string) error {
	var cur interface{}

	cur = m

	for i, k := range path {
		if i == len(path)-1 {
			switch typed := cur.(type) {
			case map[string]interface{}:
				typed[k] = v
			case map[interface{}]interface{}:
				typed[k] = v
			default:
				return fmt.Errorf("unexpected type: key=%v, value=%v, type=%T", path[:i+1], typed, typed)
			}
			return nil
		} else {
			switch typed := cur.(type) {
			case map[string]interface{}:
				if _, ok := typed[k]; !ok {
					typed[k] = map[string]interface{}{}
				}
				cur = typed[k]
			case map[interface{}]interface{}:
				if _, ok :=  typed[k]; !ok {
					typed[k] = map[string]interface{}{}
				}
				cur = typed[k]
			default:
				return fmt.Errorf("unexpected type: key=%v, value=%v, type=%T", path[:i+1], typed, typed)
			}
		}
	}

	panic("invalid state")
}

func (m mapConfig) Config(path ...string) api.StaticConfig {
	return Map(m.Map(path...))
}

func (m mapConfig) Exists(path ...string) bool {
	var cur interface{}
	var ok bool

	cur = m.m

	for _, k := range path {
		switch typed := cur.(type) {
		case map[string]interface{}:
			cur, ok = typed[k]
			if !ok {
				return false
			}
		case map[interface{}]interface{}:
			cur, ok = typed[k]
			if !ok {
				return false
			}
		default:
			return false
		}
	}

	return true
}

func (m mapConfig) Map(path ...string) map[string]interface{} {
	var cur interface{}

	cur = m.m

	for _, k := range path {
		switch typed := cur.(type) {
		case map[string]interface{}:
			cur = typed[k]
		case map[interface{}]interface{}:
			cur = typed[k]
		default:
			return nil
		}
	}

	switch typed := cur.(type) {
	case map[string]interface{}:
		return typed
	case map[interface{}]interface{}:
		strmap := map[string]interface{}{}
		for k, v := range typed {
			strmap[fmt.Sprintf("%v", k)] = v
		}
		return strmap
	default:
		return nil
	}

	panic("invalid state")
}

func Map(m map[string]interface{}) mapConfig {
	return mapConfig{
		m: m,
	}
}
