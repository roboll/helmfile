package helmfile

import (
	"bytes"
	"os"
	"testing"
	"text/template"

	"github.com/roboll/helmfile/pkg/tmpl"
	"github.com/stretchr/testify/require"
)

type tmplTestCase struct {
	//  envs are set in the test environment
	envs map[string]string
	// name of the test
	name string
	// tmplString is the template string to be parsed
	tmplString string
	// data is the data to be passed to the template
	data interface{}
	// wantErr is true if the template should fail to parse
	wantErr bool
	// output is the expected output of the template
	output string
}

// setEnvs sets the environment variables for the test case
func (t *tmplTestCase) setEnvs() {
	for k, v := range t.envs {
		os.Setenv(k, v)
	}
}

// unSetEnvs unsets the environment variables for the test case
func (t *tmplTestCase) unSetEnvs() {
	for k := range t.envs {
		os.Unsetenv(k)
	}
}

// tmplTestCases are the test cases for the template tests
var tmplTestCases = []tmplTestCase{
	{
		envs: map[string]string{
			"TEST_VAR": "test",
		},
		name:       "requiredEnvWithEnvs",
		tmplString: `{{ requiredEnv "TEST_VAR" }}`,
		output:     "test",
	},
	{
		name:       "requiredEnv",
		tmplString: `{{ requiredEnv "TEST_VAR" }}`,
		wantErr:    true,
	},
	{
		name:       "requiredWithEmptyString",
		tmplString: `{{ "" | required "required" }}`,
		wantErr:    true,
	},
	{
		name:       "required",
		tmplString: `{{ "test" | required "required" }}`,
		output:     "test",
	},
}

// TestTmplStrings tests the template string
func TestTmplStrings(t *testing.T) {
	c := &tmpl.Context{}
	tmpl := template.New("stringTemplateTest").Funcs(c.CreateFuncMap())

	for _, tc := range tmplTestCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setEnvs()
			defer tc.unSetEnvs()
			tmpl, err := tmpl.Parse(tc.tmplString)
			require.Nilf(t, err, "error parsing template: %v", err)

			var tplResult bytes.Buffer
			err = tmpl.Execute(&tplResult, tc.data)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.output, tplResult.String())
		})
	}

}
