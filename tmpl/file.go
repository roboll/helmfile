package tmpl

import (
	"bytes"
	"github.com/roboll/helmfile/environment"
)

type templateFileRenderer struct {
	ReadFile func(string) ([]byte, error)
	Context  *Context
	Data     TemplateData
}

type TemplateData struct {
	// Environment is accessible as `.Environment` from any template executed by the renderer
	Environment environment.Environment
}

type FileRenderer interface {
	RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error)
}

func NewFileRenderer(readFile func(filename string) ([]byte, error), basePath string, env environment.Environment) *templateFileRenderer {
	return &templateFileRenderer{
		ReadFile: readFile,
		Context: &Context{
			basePath: basePath,
			readFile: readFile,
		},
		Data: TemplateData{
			Environment: env,
		},
	}
}

func (r *templateFileRenderer) RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error) {
	content, err := r.ReadFile(file)
	if err != nil {
		return nil, err
	}

	return r.Context.RenderTemplateToBuffer(string(content), r.Data)
}
