package args

import (
	"fmt"
	"strings"

	"github.com/roboll/helmfile/state"
)

type argMap struct {
	m     map[string]string
	flags []string
}

func (a *argMap) SetArg(flag, arg string) {
	if _, exists := a.m[flag]; !exists {
		a.m[flag] = arg
		a.flags = append(a.flags, flag)
	}
}

func newArgMap() *argMap {
	return &argMap{m: map[string]string{}}
}

func GetArgs(args string, state *state.HelmState) []string {
	//args := c.String("args")
	argsMap := newArgMap()
	spaceflagArg := map[string]bool{}

	if len(args) > 0 {
		argsVals := strings.Split(args, " ")
		prevFlag := ""
		for _, arg := range argsVals {
			if strings.HasPrefix(arg, "--") {
				argVal := strings.SplitN(arg, "=", 2)
				if len(argVal) > 1 {
					arg := argVal[0]
					value := argVal[1]
					argsMap.SetArg(arg, value)
				} else {
					argsMap.SetArg(arg, "")
				}
				prevFlag = arg
			} else {
				spaceflagArg[prevFlag] = true
				argsMap.m[prevFlag] = arg
			}
		}
	}

	if len(state.HelmDefaults.Args) > 0 {
		for _, arg := range state.HelmDefaults.Args {
			var flag string
			var val string

			argsNum, _ := fmt.Sscanf(arg, "--%s %s", &flag, &val)
			if argsNum == 2 {
				argsMap.SetArg(flag, arg)
			} else {
				argVal := strings.SplitN(arg, "=", 2)
				argFirst := argVal[0]
				if len(argVal) > 1 {
					val = argVal[1]
					argsMap.SetArg(argFirst, val)
				} else {
					argsMap.SetArg(argFirst, "")
				}
			}
		}
	}

	if state.HelmDefaults.TillerNamespace != "" {
		argsMap.SetArg("--tiller-namespace", state.HelmDefaults.TillerNamespace)
	}
	if state.HelmDefaults.KubeContext != "" {
		argsMap.SetArg("--kube-context", state.HelmDefaults.KubeContext)
	}

	var argArr []string

	for _, flag := range argsMap.flags {
		val := argsMap.m[flag]

		if val != "" {
			if spaceflagArg[flag] {
				argArr = append(argArr, flag, val)
			} else {
				argArr = append(argArr, fmt.Sprintf("%s=%s", flag, val))
			}

		} else {
			argArr = append(argArr, fmt.Sprintf("%s", flag))
		}
	}

	state.HelmDefaults.Args = argArr

	return state.HelmDefaults.Args
}
