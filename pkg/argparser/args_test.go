package argparser

import (
	"strings"
	"testing"

	"github.com/roboll/helmfile/pkg/state"
)

func TestGetArgs(t *testing.T) {
	args := "--timeout=3600 --set app1.bootstrap=true --set app2.bootstrap=false --tiller-namespace ns"
	defaultArgs := []string{"--recreate-pods", "--force"}
	Helmdefaults := state.HelmSpec{KubeContext: "test", TillerNamespace: "test-namespace", Args: defaultArgs}
	testState := &state.HelmState{
		ReleaseSetSpec: state.ReleaseSetSpec{
			HelmDefaults: Helmdefaults,
		},
	}
	receivedArgs := GetArgs(args, testState)

	expectedOutput := "--timeout=3600 --set app1.bootstrap=true --set app2.bootstrap=false --tiller-namespace ns --recreate-pods --force"

	if compareArgs(expectedOutput, receivedArgs) == false {
		t.Errorf("expected %s, got %s", expectedOutput, strings.Join(receivedArgs, " "))
	}
}

func Test2(t *testing.T) {
	args := "--timeout=3600 --set app1.bootstrap=true --set app2.bootstrap=false,app3.bootstrap=true --tiller-namespace ns"
	defaultArgs := []string{"--recreate-pods", "--force"}
	Helmdefaults := state.HelmSpec{KubeContext: "test", TillerNamespace: "test-namespace", Args: defaultArgs}
	testState := &state.HelmState{
		ReleaseSetSpec: state.ReleaseSetSpec{
			HelmDefaults: Helmdefaults,
		},
	}
	receivedArgs := GetArgs(args, testState)

	expectedOutput := "--timeout=3600 --set app1.bootstrap=true --set app2.bootstrap=false,app3.bootstrap=true --tiller-namespace ns --recreate-pods --force"

	if compareArgs(expectedOutput, receivedArgs) == false {
		t.Errorf("expected %s, got %s", expectedOutput, strings.Join(receivedArgs, " "))
	}

}

func compareArgs(expectedArgs string, args []string) bool {
	return strings.Compare(strings.Join(args, " "), expectedArgs) == 0
}
