package tmpl

import (
	"bytes"
)

type templateFileRenderer struct {
	ReadFile func(string) ([]byte, error)
	Context  *Context
}

type FileRenderer interface {
	RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error)
}

func NewFileRenderer(readFile func(filename string) ([]byte, error), basePath string) *templateFileRenderer {
	return &templateFileRenderer{
		ReadFile: readFile,
		Context: &Context{
			basePath: basePath,
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
