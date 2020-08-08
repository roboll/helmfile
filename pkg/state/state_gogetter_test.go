package state

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/roboll/helmfile/pkg/helmexec"
	"io/ioutil"
	"os"
	"testing"
)

func TestGoGetter(t *testing.T) {
	logger := helmexec.NewLogger(os.Stderr, "warn")

	testcases := []struct {
		chart, dir string
		force      bool

		out, err string
	}{
		{
			chart: "raw/incubator",
			dir:   "",
			force: false,
			out:   "raw/incubator",
			err:   "",
		},
	}

	for i, tc := range testcases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			d, err := ioutil.TempDir("", "testgogetter")
			if err != nil {
				panic(err)
			}
			defer os.RemoveAll(d)

			st := &HelmState{
				logger:   logger,
				readFile: ioutil.ReadFile,
				basePath: d,
			}

			out, err := st.goGetterChart(tc.chart, tc.dir, false)

			if diff := cmp.Diff(tc.out, out); diff != "" {
				t.Fatalf("Unexpected out:\n%s", diff)
			}

			var errMsg string

			if err != nil {
				errMsg = err.Error()
			}

			if diff := cmp.Diff(tc.err, errMsg); diff != "" {
				t.Fatalf("Unexpected err:\n%s", diff)
			}
		})
	}
}
