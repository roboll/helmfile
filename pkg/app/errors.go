package app

import (
	"fmt"
	"strings"
)

type NoMatchingHelmfileError struct {
	selectors []string
	env       string
}

func (e *NoMatchingHelmfileError) Error() string {
	return fmt.Sprintf(
		"err: no releases found that matches specified selector(%s) and environment(%s), in any helmfile",
		strings.Join(e.selectors, ", "),
		e.env,
	)
}
