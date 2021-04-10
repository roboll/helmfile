package event

import (
	"fmt"
	"strings"

	"github.com/roboll/helmfile/pkg/environment"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/tmpl"
	"go.uber.org/zap"
)

type Hook struct {
	Name     string            `yaml:"name"`
	Events   []string          `yaml:"events"`
	Command  string            `yaml:"command"`
	Kubectl  map[string]string `yaml:"kubectlApply,omitempty"`
	Args     []string          `yaml:"args"`
	ShowLogs bool              `yaml:"showlogs"`
}

type event struct {
	Name  string
	Error error
}

type Bus struct {
	Runner helmexec.Runner
	Hooks  []Hook

	BasePath      string
	StateFilePath string
	Namespace     string
	Chart         string

	Env environment.Environment

	ReadFile func(string) ([]byte, error)
	Logger   *zap.SugaredLogger
}

func (bus *Bus) Trigger(evt string, evtErr error, context map[string]interface{}) (bool, error) {
	if bus.Runner == nil {
		bus.Runner = helmexec.ShellRunner{
			Dir:    bus.BasePath,
			Logger: bus.Logger,
		}
	}

	executed := false

	for _, hook := range bus.Hooks {
		contained := false
		for _, e := range hook.Events {
			contained = contained || e == evt
		}
		if !contained {
			continue
		}

		var err error

		name := hook.Name
		if name == "" {
			if hook.Kubectl != nil {
				name = "kubectlApply"
			} else {
				name = hook.Command
			}
		}

		if hook.Kubectl != nil {
			if hook.Command != "" {
				bus.Logger.Warnf("warn: ignoring command '%s' given within a kubectlApply hook", hook.Command)
			}
			hook.Command = "kubectl"
			if val, found := hook.Kubectl["filename"]; found {
				if _, found := hook.Kubectl["kustomize"]; found {
					return false, fmt.Errorf("hook[%s]: kustomize & filename cannot be used together", name)
				}
				hook.Args = append([]string{"apply", "-f"}, val)
			} else if val, found := hook.Kubectl["kustomize"]; found {
				hook.Args = append([]string{"apply", "-k"}, val)
			} else {
				return false, fmt.Errorf("hook[%s]: either kustomize or filename must be given", name)
			}
		}

		bus.Logger.Debugf("hook[%s]: stateFilePath=%s, basePath=%s\n", name, bus.StateFilePath, bus.BasePath)

		data := map[string]interface{}{
			"Environment": bus.Env,
			"Namespace":   bus.Namespace,
			"Event": event{
				Name:  evt,
				Error: evtErr,
			},
		}
		for k, v := range context {
			data[k] = v
		}
		render := tmpl.NewTextRenderer(bus.ReadFile, bus.BasePath, data)

		bus.Logger.Debugf("hook[%s]: triggered by event \"%s\"\n", name, evt)

		command, err := render.RenderTemplateText(hook.Command)
		if err != nil {
			return false, fmt.Errorf("hook[%s]: %v", name, err)
		}

		args := make([]string, len(hook.Args))
		for i, raw := range hook.Args {
			args[i], err = render.RenderTemplateText(raw)
			if err != nil {
				return false, fmt.Errorf("hook[%s]: %v", name, err)
			}
		}

		bytes, err := bus.Runner.Execute(command, args, map[string]string{})
		bus.Logger.Debugf("hook[%s]: %s\n", name, string(bytes))
		if hook.ShowLogs {
			prefix := fmt.Sprintf("\nhook[%s] logs | ", evt)
			bus.Logger.Infow(prefix + strings.ReplaceAll(string(bytes), "\n", prefix))
		}

		if err != nil {
			return false, fmt.Errorf("hook[%s]: command `%s` failed: %v", name, command, err)
		}

		executed = true
	}

	return executed, nil
}
