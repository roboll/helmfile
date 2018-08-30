package tmpl

import (
	"bytes"
	"io/ioutil"
)

var DefaultFileRenderer *templateFileRenderer

func init() {
	DefaultFileRenderer = NewFileRenderer(ioutil.ReadFile)
}

type templateFileRenderer struct {
	ReadFile func(string) ([]byte, error)
	Context  *Context
}

type FileRenderer interface {
	RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error)
}

func NewFileRenderer(readFile func(filename string) ([]byte, error)) *templateFileRenderer {
	return &templateFileRenderer{
		ReadFile: readFile,
		Context: &Context{
			readFile: readFile,
		},
	}
}

func (r *templateFileRenderer) RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error) {
	content, err := r.ReadFile(file)
	if err != nil {
		return nil, err
	}

	return r.Context.RenderTemplateToBuffer(string(content))
}

func RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error) {
	return DefaultFileRenderer.RenderTemplateFileToBuffer(file)
}
