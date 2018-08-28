package tmpl

import "io/ioutil"

var DefaultContext *Context

func init() {
	DefaultContext = &Context{
		readFile: ioutil.ReadFile,
	}
}

type Context struct {
	readFile func(string) ([]byte, error)
}
