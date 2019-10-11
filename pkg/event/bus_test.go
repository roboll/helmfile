package event

import (
	"fmt"
	"os"
	"testing"

	"github.com/roboll/helmfile/pkg/environment"
	"github.com/roboll/helmfile/pkg/helmexec"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

var logger = helmexec.NewLogger(os.Stdout, "warn")

type runner struct {
}

func (r *runner) Execute(cmd string, args []string, env map[string]string) ([]byte, error) {
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
			&Hook{"okhook1", []string{"foo"}, "ok", []string{}, true},
			"foo",
			true,
			"",
		},
		{
			"okhooké",
			&Hook{"okhook2", []string{"foo"}, "ok", []string{}, false},
			"foo",
			true,
			"",
		},
		{
			"missinghook1",
			&Hook{"okhook1", []string{"foo"}, "ok", []string{}, false},
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
			&Hook{"nghook1", []string{"foo"}, "ng", []string{}, false},
			"foo",
			false,
			"hook[nghook1]: command `ng` failed: cmd failed due to invalid cmd: ng",
		},
		{
			"nghook2",
			&Hook{"nghook2", []string{"foo"}, "ok", []string{"ng"}, false},
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
		observer, observedLogs := observer.New(zap.InfoLevel)
		zeLogger := zap.New(observer).Sugar()
		bus := &Bus{
			Hooks:         hooks,
			StateFilePath: "path/to/helmfile.yaml",
			BasePath:      "path/to",
			Namespace:     "myns",
			Env:           environment.Environment{Name: "prod"},
			Logger:        zeLogger,
			ReadFile:      readFile,
		}

		bus.Runner = &runner{}
		data := map[string]interface{}{
			"Release":         "myrel",
			"HelmfileCommand": "mycmd",
		}
		ok, err := bus.Trigger(c.triggeredEvt, nil, data)

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
		if observedLogs.Len() != 0 && !hooks[0].ShowLogs {
			t.Errorf("unexpected error for case \"%s\": Logs should not be created : %v", c.name, observedLogs.All())
		}
	}
}
