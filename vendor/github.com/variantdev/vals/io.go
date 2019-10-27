package vals

import (
	"bufio"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"os"
)

func Inputs(f string) ([]yaml.Node, error) {
	var reader io.Reader
	if f == "-" {
		reader = os.Stdin
	} else if f != "" {
		fp, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		reader = fp
		defer fp.Close()
	} else {
		return nil, fmt.Errorf("Nothing to eval: No file specified")
	}

	nodes := []yaml.Node{}
	buf := bufio.NewReader(reader)
	decoder := yaml.NewDecoder(buf)
	for {
		node := yaml.Node{}
		if err := decoder.Decode(&node); err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func Input(f string) (map[string]interface{}, error) {
	m := map[string]interface{}{}
	var input []byte
	var err error
	if f == "-" {
		input, err = ioutil.ReadAll(os.Stdin)
	} else if f != "" {
		input, err = ioutil.ReadFile(f)
	} else {
		return nil, fmt.Errorf("Nothing to eval: No file specified")
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(input, &m); err != nil {
		return nil, err
	}

	return m, nil
}

func Output(o string, res interface{}) (*string, error) {
	var err error
	var out []byte
	switch o {
	case "yaml":
		out, err = yaml.Marshal(res)
	case "json":
		out, err = json.Marshal(res)
	default:
		return nil, fmt.Errorf("Unknown output type: %s", o)
	}
	if err != nil {
		return nil, fmt.Errorf("Failed marshalling into %s: %v", o, err)
	}
	str := string(out)
	return &str, nil
}
