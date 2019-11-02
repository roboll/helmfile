package helmexec

import (
	"fmt"
	"path/filepath"
	"strings"
)

func newExitError(helmCmdPath string, exitStatus int, errorMessage string) ExitError {
	return ExitError{
		Message: fmt.Sprintf("%s exited with status %d:\n%s", filepath.Base(helmCmdPath), exitStatus, indent(strings.TrimSpace(errorMessage))),
		Code:    exitStatus,
	}
}

func indent(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// ExitError is created whenever your shell command exits with a non-zero exit status
type ExitError struct {
	Message string
	Code    int
}

func (e ExitError) Error() string {
	return e.Message
}

func (e ExitError) ExitStatus() int {
	return e.Code
}
