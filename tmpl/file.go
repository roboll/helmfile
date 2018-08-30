package tmpl

import (
	"bytes"
	"path/filepath"
)

type templateFileRenderer struct {
	basePath string
	ReadFile func(string) ([]byte, error)
	Context  *Context
}

type FileRenderer interface {
	RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error)
}

func NewFileRenderer(readFile func(filename string) ([]byte, error), basePath string) *templateFileRenderer {
	return &templateFileRenderer{
		basePath: basePath,
		ReadFile: readFile,
		Context: &Context{
			readFile: readFile,
		},
	}
}

func (r *templateFileRenderer) RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error) {
	// path to the file relative to the helmfile.yaml
	path := filepath.Join(r.basePath, file)

	content, err := r.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return r.Context.RenderTemplateToBuffer(string(content))
}
