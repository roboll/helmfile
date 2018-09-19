package event

import (
	"fmt"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/helmexec"
	"os"
	"testing"
)

var logger = helmexec.NewLogger(os.Stdout, "warn")

type runner struct {
}

func (r *runner) Execute(cmd string, args []string) ([]byte, error) {
	if cmd == "ng" {
		return nil, fmt.Errorf("cmd failed due to invalid cmd: %s", cmd)
	}
	for _, a := range args {
		if a == "ng" {
			return nil, fmt.Errorf("cmd failed due to invalid arg: %s", a)
		}
	}
	return []byte(""), nil
}

func TestTrigger(t *testing.T) {
	cases := []struct {
		name           string
		hook           *Hook
		triggeredEvt   string
		expectedResult bool
		expectedErr    string
	}{
		{
			"okhook1",
			&Hook{"okhook1", []string{"foo"}, "ok", []string{}},
			"foo",
			true,
			"",
		},
		{
			"missinghook1",
			&Hook{"okhook1", []string{"foo"}, "ok", []string{}},
			"bar",
			false,
			"",
		},
		{
			"nohook1",
			nil,
			"bar",
			false,
			"",
		},
		{
			"nghook1",
			&Hook{"nghook1", []string{"foo"}, "ng", []string{}},
			"foo",
			false,
			"hook[nghook1]: command `ng` failed: cmd failed due to invalid cmd: ng",
		},
		{
			"nghook2",
			&Hook{"nghook2", []string{"foo"}, "ok", []string{"ng"}},
			"foo",
			false,
			"hook[nghook2]: command `ok` failed: cmd failed due to invalid arg: ng",
		},
	}
	readFile := func(filename string) ([]byte, error) {
		return nil, fmt.Errorf("unexpected call to readFile: %s", filename)
	}
	for _, c := range cases {
		hooks := []Hook{}
		if c.hook != nil {
			hooks = append(hooks, *c.hook)
		}
		bus := &Bus{
			Hooks:         hooks,
			StateFilePath: "path/to/helmfile.yaml",
			BasePath:      "path/to",
			Namespace:     "myns",
			Env:           environment.Environment{Name: "prod"},
			Logger:        logger,
			ReadFile:      readFile,
		}

		bus.Runner = &runner{}
		data := map[string]interface{}{
			"Release":         "myrel",
			"HelmfileCommand": "mycmd",
		}
		ok, err := bus.Trigger(c.triggeredEvt, data)

		if ok != c.expectedResult {
			t.Errorf("unexpected result for case \"%s\": expected=%v, actual=%v", c.name, c.expectedResult, ok)
		}

		if c.expectedErr != "" {
			if err == nil {
				t.Error("error expected, but not occurred")
			} else if err.Error() != c.expectedErr {
				t.Errorf("unexpected error for case \"%s\": expected=%s, actual=%v", c.name, c.expectedErr, err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for case \"%s\": %v", c.name, err)
			}
		}
	}
}
