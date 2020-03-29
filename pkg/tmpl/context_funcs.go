package tmpl

import (
	"fmt"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
)

type Values = map[string]interface{}

func (c *Context) createFuncMap() template.FuncMap {
	funcMap := template.FuncMap{
		"exec":           c.Exec,
		"readFile":       c.ReadFile,
		"toYaml":         ToYaml,
		"fromYaml":       FromYaml,
		"setValueAtPath": SetValueAtPath,
		"requiredEnv":    RequiredEnv,
		"get":            get,
		"getOrNil":       getOrNil,
		"tpl":            c.Tpl,
	}
	if c.preRender {
		// disable potential side-effect template calls
		funcMap["exec"] = func(string, []interface{}, ...string) (string, error) {
			return "", nil
		}
		funcMap["readFile"] = func(string) (string, error) {
			return "", nil
		}
	}

	return funcMap
}

func (c *Context) Exec(command string, args []interface{}, inputs ...string) (string, error) {
	var input string
	if len(inputs) > 0 {
		input = inputs[0]
	}

	strArgs := make([]string, len(args))
	for i, a := range args {
		switch a.(type) {
		case string:
			strArgs[i] = a.(string)
		default:
			return "", fmt.Errorf("unexpected type of arg \"%s\" in args %v at index %d", reflect.TypeOf(a), args, i)
		}
	}

	cmd := exec.Command(command, strArgs...)
	cmd.Dir = c.basePath

	g := errgroup.Group{}

	if len(input) > 0 {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", err
		}

		g.Go(func() error {
			defer stdin.Close()

			size := len(input)

			i := 0

			for {
				n, err := io.WriteString(stdin, input[i:])
				if err != nil {
					return fmt.Errorf("failed while writing %d bytes to stdin of \"%s\": %v", len(input), command, err)
				}

				i += n

				if i == size {
					return nil
				}
			}
		})
	}

	var bytes []byte

	g.Go(func() error {
		// We use CombinedOutput to produce helpful error messages
		// See https://github.com/roboll/helmfile/issues/1158
		bs, err := cmd.CombinedOutput()
		if err != nil {
			args := strings.Join(strArgs, ", ")
			shownCmd := []string{command}
			if len(args) > 0 {
				shownCmd = append(shownCmd, args)
			}

			var out string

			out += fmt.Sprintf("\n\nCOMMAND:\n%s", Indent(strings.Join(shownCmd, " "), "  "))

			out += fmt.Sprintf("\n\nERROR:\n%s", Indent(err.Error(), "  "))

			if len(bs) > 0 {
				out += fmt.Sprintf("\n\nCOMBINED OUTPUT:\n%s", Indent(string(bs), "  "))
			}

			return fmt.Errorf("%v%s", err, out)
		}

		bytes = bs

		return nil
	})

	if err := g.Wait(); err != nil {
		return "", err
	}

	return string(bytes), nil
}

// indents a block of text with an indent string
func Indent(text, indent string) string {
	var b strings.Builder

	b.Grow(len(text) * 2)

	lines := strings.Split(text, "\n")

	last := len(lines) - 1

	for i, j := range lines {
		if i > 0 && i < last && j != "" {
			b.WriteString("\n")
		}

		if j != "" {
			b.WriteString(indent + j)
		}
	}

	return b.String()
}

func (c *Context) ReadFile(filename string) (string, error) {
	var path string
	if filepath.IsAbs(filename) {
		path = filename
	} else {
		path = filepath.Join(c.basePath, filename)
	}

	bytes, err := c.readFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (c *Context) Tpl(text string, data interface{}) (string, error) {
	buf, err := c.RenderTemplateToBuffer(text, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func ToYaml(v interface{}) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FromYaml(str string) (Values, error) {
	m := Values{}

	if err := yaml.Unmarshal([]byte(str), &m); err != nil {
		return nil, fmt.Errorf("%s, offending yaml: %s", err, str)
	}
	return m, nil
}

func SetValueAtPath(path string, value interface{}, values Values) (Values, error) {
	var current interface{}
	current = values
	components := strings.Split(path, ".")
	pathToMap := components[:len(components)-1]
	key := components[len(components)-1]
	for _, k := range pathToMap {
		var elem interface{}

		switch typedCurrent := current.(type) {
		case map[string]interface{}:
			v, exists := typedCurrent[k]
			if !exists {
				return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" does not exist", path, k)
			}
			elem = v
		case map[interface{}]interface{}:
			v, exists := typedCurrent[k]
			if !exists {
				return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" does not exist", path, k)
			}
			elem = v
		default:
			return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" was not a map", path, k)
		}

		switch typedElem := elem.(type) {
		case map[string]interface{}, map[interface{}]interface{}:
			current = typedElem
		default:
			return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" was not a map", path, k)
		}
	}

	switch typedCurrent := current.(type) {
	case map[string]interface{}:
		typedCurrent[key] = value
	case map[interface{}]interface{}:
		typedCurrent[key] = value
	default:
		return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" was not a map", path, key)
	}
	return values, nil
}

func RequiredEnv(name string) (string, error) {
	if val, exists := os.LookupEnv(name); exists && len(val) > 0 {
		return val, nil
	}

	return "", fmt.Errorf("required env var `%s` is not set", name)
}
