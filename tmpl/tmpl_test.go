package tmpl

import (
	"fmt"
	"reflect"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	valuesYamlContent := `foo:
  bar: BAR
`
	expected := `foo:
  bar: FOO_BAR
`
	expectedFilename := "values.yaml"
	ctx := &Context{readFile: func(filename string) ([]byte, error) {
		if filename != expectedFilename {
			return nil, fmt.Errorf("unexpected filename: expected=%v, actual=%s", expectedFilename, filename)
		}
		return []byte(valuesYamlContent), nil
	}}
	buf, err := ctx.RenderTemplateToBuffer(`{{ readFile "values.yaml" | fromYaml | setValueAtPath "foo.bar" "FOO_BAR" | toYaml }}`)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actual := buf.String()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("unexpected result: expected=%v, actual=%v", expected, actual)
	}
}
