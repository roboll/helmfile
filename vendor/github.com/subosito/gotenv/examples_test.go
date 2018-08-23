package gotenv_test

import (
	"fmt"
	"github.com/subosito/gotenv"
	"strings"
)

func ExampleParse() {
	pairs := gotenv.Parse(strings.NewReader("FOO=test\nBAR=$FOO"))
	fmt.Printf("%+v\n", pairs) // gotenv.Env{"FOO": "test", "BAR": "test"}

	pairs = gotenv.Parse(strings.NewReader(`FOO="bar"`))
	fmt.Printf("%+v\n", pairs) // gotenv.Env{"FOO": "bar"}
}
