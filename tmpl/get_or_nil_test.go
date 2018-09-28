package tmpl

import (
	"testing"
)

func TestGetStruct(t *testing.T) {
	type Foo struct{ Bar string }

	obj := struct{ Foo }{Foo{Bar: "Bar"}}

	v1, err := get("Foo.Bar", obj)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v1 != "Bar" {
		t.Errorf("unexpected value for path Foo.Bar in %v: expected=Bar, actual=%v", obj, v1)
	}

	_, err = get("Foo.baz", obj)

	if err == nil {
		t.Errorf("expected error but was not occurred")
	}
}

func TestGetMap(t *testing.T) {
	obj := map[string]interface{}{"Foo": map[string]interface{}{"Bar": "Bar"}}

	v1, err := get("Foo.Bar", obj)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v1 != "Bar" {
		t.Errorf("unexpected value for path Foo.Bar in %v: expected=Bar, actual=%v", obj, v1)
	}

	_, err = get("Foo.baz", obj)

	if err == nil {
		t.Errorf("expected error but was not occurred")
	}
}

func TestGetOrNilStruct(t *testing.T) {
	type Foo struct{ Bar string }

	obj := struct{ Foo }{Foo{Bar: "Bar"}}

	v1, err := getOrNil("Foo.Bar", obj)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v1 != "Bar" {
		t.Errorf("unexpected value for path Foo.Bar in %v: expected=Bar, actual=%v", obj, v1)
	}

	v2, err := getOrNil("Foo.baz", obj)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v2 != nil {
		t.Errorf("unexpected value for path Foo.baz in %v: expected=nil, actual=%v", obj, v2)
	}
}

func TestGetOrNilMap(t *testing.T) {
	obj := map[string]interface{}{"Foo": map[string]interface{}{"Bar": "Bar"}}

	v1, err := getOrNil("Foo.Bar", obj)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v1 != "Bar" {
		t.Errorf("unexpected value for path Foo.Bar in %v: expected=Bar, actual=%v", obj, v1)
	}

	v2, err := getOrNil("Foo.baz", obj)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v2 != nil {
		t.Errorf("unexpected value for path Foo.baz in %v: expected=nil, actual=%v", obj, v2)
	}
}
