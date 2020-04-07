package tmpl

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestRenderTemplate_Values(t *testing.T) {
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

func TestRenderTemplate_WithData(t *testing.T) {
	valuesYamlContent := `foo:
  bar: {{ .foo.bar }}
`
	expected := `foo:
  bar: FOO_BAR
`
	expectedFilename := "values.yaml"
	data := map[string]interface{}{
		"foo": map[string]interface{}{
			"bar": "FOO_BAR",
		},
	}
	ctx := &Context{readFile: func(filename string) ([]byte, error) {
		if filename != expectedFilename {
			return nil, fmt.Errorf("unexpected filename: expected=%v, actual=%s", expectedFilename, filename)
		}
		return []byte(valuesYamlContent), nil
	}}
	buf, err := ctx.RenderTemplateToBuffer(valuesYamlContent, data)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actual := buf.String()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("unexpected result: expected=%v, actual=%v", expected, actual)
	}
}

func TestRenderTemplate_AccessingMissingKeyWithGetOrNil(t *testing.T) {
	valuesYamlContent := `foo:
  bar: {{ . | getOrNil "foo.bar" }}
`
	expected := `foo:
  bar: <no value>
`
	expectedFilename := "values.yaml"
	data := map[string]interface{}{}
	ctx := &Context{readFile: func(filename string) ([]byte, error) {
		if filename != expectedFilename {
			return nil, fmt.Errorf("unexpected filename: expected=%v, actual=%s", expectedFilename, filename)
		}
		return []byte(valuesYamlContent), nil
	}}
	buf, err := ctx.RenderTemplateToBuffer(valuesYamlContent, data)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actual := buf.String()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("unexpected result: expected=%v, actual=%v", expected, actual)
	}
}

func TestRenderTemplate_Defaulting(t *testing.T) {
	valuesYamlContent := `foo:
  bar: {{ . | getOrNil "foo.bar" | default "DEFAULT" }}
`
	expected := `foo:
  bar: DEFAULT
`
	expectedFilename := "values.yaml"
	data := map[string]interface{}{}
	ctx := &Context{readFile: func(filename string) ([]byte, error) {
		if filename != expectedFilename {
			return nil, fmt.Errorf("unexpected filename: expected=%v, actual=%s", expectedFilename, filename)
		}
		return []byte(valuesYamlContent), nil
	}}
	buf, err := ctx.RenderTemplateToBuffer(valuesYamlContent, data)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actual := buf.String()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("unexpected result: expected=%v, actual=%v", expected, actual)
	}
}

func renderTemplateToString(s string, data ...interface{}) (string, error) {
	ctx := &Context{readFile: func(filename string) ([]byte, error) {
		return nil, fmt.Errorf("unexpected call to readFile: filename=%s", filename)
	}}
	tplString, err := ctx.RenderTemplateToBuffer(s, data...)
	if err != nil {
		return "", err
	}
	return tplString.String(), nil
}

func Test_renderTemplateToString(t *testing.T) {
	type args struct {
		s    string
		envs map[string]string
		data interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "simple replacement",
			args: args{
				s: "{{ env \"HF_TEST_VAR\" }}",
				envs: map[string]string{
					"HF_TEST_VAR": "content",
				},
			},
			want:    "content",
			wantErr: false,
		},
		{
			name: "two replacements",
			args: args{
				s: "{{ env \"HF_TEST_ALPHA\" }}{{ env \"HF_TEST_BETA\" }}",
				envs: map[string]string{
					"HF_TEST_ALPHA": "first",
					"HF_TEST_BETA":  "second",
				},
			},
			want:    "firstsecond",
			wantErr: false,
		},
		{
			name: "replacement and comment",
			args: args{
				s: "{{ env \"HF_TEST_ALPHA\" }}{{/* comment */}}",
				envs: map[string]string{
					"HF_TEST_ALPHA": "first",
				},
			},
			want:    "first",
			wantErr: false,
		},
		{
			name: "global template function",
			args: args{
				s: "{{ env \"HF_TEST_ALPHA\" | len }}",
				envs: map[string]string{
					"HF_TEST_ALPHA": "abcdefg",
				},
			},
			want:    "7",
			wantErr: false,
		},
		{
			name: "get",
			args: args{
				s:    `{{ . | get "Foo" }}, {{ . | get "Bar" "2" }}`,
				envs: map[string]string{},
				data: map[string]interface{}{
					"Foo": "1",
				},
			},
			want:    "1, 2",
			wantErr: false,
		},
		{
			name: "env var not set",
			args: args{
				s: "{{ env \"HF_TEST_NONE\" }}",
				envs: map[string]string{
					"HF_TEST_THIS": "first",
				},
			},
			want: "",
		},
		{
			name: "undefined function",
			args: args{
				s: "{{ env foo }}",
				envs: map[string]string{
					"foo": "bar",
				},
			},
			wantErr: true,
		},
		{
			name: "required env var",
			args: args{
				s: "{{ requiredEnv \"HF_TEST\" }}",
				envs: map[string]string{
					"HF_TEST": "value",
				},
			},
			want:    "value",
			wantErr: false,
		},
		{
			name: "required env var not set",
			args: args{
				s:    "{{ requiredEnv \"HF_TEST_NONE\" }}",
				envs: map[string]string{},
			},
			wantErr: true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.args.envs {
				err := os.Setenv(k, v)
				if err != nil {
					t.Error("renderTemplateToString() could not set env var for testing")
				}
			}
			got, err := renderTemplateToString(tt.args.s, tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderTemplateToString() for %s error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("renderTemplateToString() for %s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestRenderTemplate_Required(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		data    map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: ".foo is existed",
			s:    `{{ required ".foo.bar is required" .foo }}`,
			data: map[string]interface{}{
				"foo": "bar",
			},
			want:    "bar",
			wantErr: false,
		},
		{
			name: ".foo.bar is existed",
			s:    `{{ required "foo.bar is required" .foo.bar }}`,
			data: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "FOO_BAR",
				},
			},
			want:    "FOO_BAR",
			wantErr: false,
		},
		{
			name: ".foo.bar is existed but value is nil",
			s:    `{{ required "foo.bar is required" .foo.bar }}`,
			data: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			wantErr: true,
		},
		{
			name: ".foo.bar is existed but value is empty string",
			s:    `{{ required "foo.bar is required" .foo.bar }}`,
			data: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "",
				},
			},
			wantErr: true,
		},
		{
			name: ".foo is nil",
			s:    `{{ required "foo is required" .foo }}`,
			data: map[string]interface{}{
				"foo": nil,
			},
			wantErr: true,
		},
		{
			name: ".foo is a empty string",
			s:    `{{ required "foo is required" .foo }}`,
			data: map[string]interface{}{
				"foo": "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		got, err := renderTemplateToString(tt.s, tt.data)
		if (err != nil) != tt.wantErr {
			t.Errorf("renderTemplateToString() for %s error = %v, wantErr %v", tt.name, err, tt.wantErr)
			return
		}
		if got != tt.want {
			t.Errorf("renderTemplateToString() for %s = %v, want %v", tt.name, got, tt.want)
		}
	}
}
