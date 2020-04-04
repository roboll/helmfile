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
	return e.err.Error()
}

func NewReleaseError(release *ReleaseSpec, err error, code int) *ReleaseError {
	return &ReleaseError{
		ReleaseSpec: release,
		err:         err,
		Code:        code,
	}
}

func newReleaseFailedError(release *ReleaseSpec, err error) *ReleaseError {
	wrappedErr := fmt.Errorf("failed processing release %s: %v", release.Name, err.Error())

	return NewReleaseError(release, wrappedErr, ReleaseErrorCodeFailure)
}
