package args

import (
	"fmt"
	"strings"
	"testing"

	"github.com/roboll/helmfile/state"
)

func TestGetArgs(t *testing.T) {
	args := "--timeout=3600 --set app1.bootstrap=true --set app2.bootstrap=false --tiller-namespace ns"
	defaultArgs := []string{"--recreate-pods", "--force"}
	fmt.Println(defaultArgs)
	fmt.Println(len(defaultArgs))
	Helmdefaults := state.HelmSpec{KubeContext: "test", TillerNamespace: "test-namespace", Args: defaultArgs}
	testState := &state.HelmState{HelmDefaults: Helmdefaults}
	receivedArgs := GetArgs(args, testState)

	expectedOutput := "--timeout=3600 --set app1.bootstrap=true --set app2.bootstrap=false --tiller-namespace ns --recreate-pods --force --kube-context=test"

	if compareArgs(expectedOutput, receivedArgs) == false {
		t.Errorf("expected %s, got %s", expectedOutput, strings.Join(receivedArgs, " "))
	}
}

func compareArgs(expectedArgs string, args []string) bool {

	if strings.Compare(strings.Join(args, " "), expectedArgs) != 0 {
		return false
	}
	return true

}
