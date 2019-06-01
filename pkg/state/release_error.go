package state

import (
	"fmt"
)

const ReleaseErrorCodeFailure = 1

type ReleaseError struct {
	*ReleaseSpec
	err  error
	Code int
}

func (e *ReleaseError) Error() string {
	return fmt.Sprintf("failed processing release %s: %v", e.Name, e.err.Error())
}

func newReleaseError(release *ReleaseSpec, err error) *ReleaseError {
	return &ReleaseError{release, err, ReleaseErrorCodeFailure}
}
