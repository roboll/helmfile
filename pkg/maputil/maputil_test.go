package maputil

import "testing"

func TestMapUtil_StrKeys(t *testing.T) {
	m := map[string]interface{}{
		"a": []interface{}{
			map[string]interface{}{
				"b": []interface{}{
					map[string]interface{}{
						"c": "C",
					},
				},
			},
		},
	}

	r, err := CastKeysToStrings(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := r["a"].([]interface{})
	a0 := a[0].(map[string]interface{})
	b := a0["b"].([]interface{})
	b0 := b[0].(map[string]interface{})
	c := b0["c"]

	if c != "C" {
		t.Errorf("unexpected c: expected=C, got=%s", c)
	}
}

func TestMapUtil_IFKeys(t *testing.T) {
	m := map[interface{}]interface{}{
		"a": []interface{}{
			map[interface{}]interface{}{
				"b": []interface{}{
					map[interface{}]interface{}{
						"c": "C",
					},
				},
			},
		},
	}

	r, err := CastKeysToStrings(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := r["a"].([]interface{})
	a0 := a[0].(map[string]interface{})
	b := a0["b"].([]interface{})
	b0 := b[0].(map[string]interface{})
	c := b0["c"]

	if c != "C" {
		t.Errorf("unexpected c: expected=C, got=%s", c)
	}
}
