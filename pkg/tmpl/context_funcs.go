package tmpl

import (
	"fmt"
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

	writeErrs := make(chan error)
	cmdErrs := make(chan error)
	cmdOuts := make(chan []byte)

	if len(input) > 0 {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", err
		}
		go func(input string, stdin io.WriteCloser) {
			defer stdin.Close()
			defer close(writeErrs)

			size := len(input)

			var n int
			var err error
			i := 0
			for {
				n, err = io.WriteString(stdin, input[i:])
				if err != nil {
					writeErrs <- fmt.Errorf("failed while writing %d bytes to stdin of \"%s\": %v", len(input), command, err)
					break
				}
				i += n
				if n == size {
					break
				}
			}
		}(input, stdin)
	}

	go func() {
		defer close(cmdOuts)
		defer close(cmdErrs)

		bytes, err := cmd.Output()
		if err != nil {
			cmdErrs <- fmt.Errorf("exec cmd=%s args=[%s] failed: %v", command, strings.Join(strArgs, ", "), err)
		} else {
			cmdOuts <- bytes
		}
	}()

	for {
		select {
		case bytes := <-cmdOuts:
			return string(bytes), nil
		case err := <-cmdErrs:
			return "", err
		case err := <-writeErrs:
			return "", err
		}
	}
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
