package app

import (
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	"gotest.tools/env"
	"os"
	"testing"
)

func TestIsExplicitSelectorInheritanceEnabled(t *testing.T) {
	//env var ExperimentalEnvVar is set
	assert.Assert(t, is.Equal(os.Getenv(ExperimentalEnvVar), ""))
	assert.Check(t, !isExplicitSelectorInheritanceEnabled())

	//check for env var ExperimentalEnvVar set to true
	defer env.Patch(t, ExperimentalEnvVar, "true")()
	assert.Check(t, isExplicitSelectorInheritanceEnabled())

	//check for env var ExperimentalEnvVar set to anything
	defer env.Patch(t, ExperimentalEnvVar, "foo")()
	assert.Check(t, !isExplicitSelectorInheritanceEnabled())

	//check for env var ExperimentalEnvVar set to ExperimentalSelectorExplicit
	defer env.Patch(t, ExperimentalEnvVar, ExperimentalSelectorExplicit)()
	assert.Check(t, isExplicitSelectorInheritanceEnabled())
}
