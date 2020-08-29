package helmexec

import "math/rand"

var executionIDComponents = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func newExecutionID() string {
	b := make([]rune, 5)
	for i := range b {
		b[i] = executionIDComponents[rand.Intn(len(executionIDComponents))]
	}
	return string(b)
}
