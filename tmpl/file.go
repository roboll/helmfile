package tmpl

import (
	"bytes"
	"io/ioutil"

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
        Namespace string
}

type FileRenderer interface {
	RenderTemplateFileToBuffer(file string) (*bytes.Buffer, error)
}

func NewFileRenderer(readFile func(filename string) ([]byte, error), basePath string, env environment.Environment, namespace string) *templateFileRenderer {
	return &templateFileRenderer{
		ReadFile: readFile,
		Context: &Context{
			basePath: basePath,
			readFile: readFile,
		},
		Data: TemplateData{
			Environment: env,
                        Namespace: namespace,
		},
	}
}

func NewFirstPassRenderer(env environment.Environment) *templateFileRenderer {
	return &templateFileRenderer{
		ReadFile: ioutil.ReadFile,
		Context: &Context{
			preRender: true,
			basePath:  "",
			readFile:  ioutil.ReadFile,
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

	return r.RenderTemplateContentToBuffer(content)
}

func (r *templateFileRenderer) RenderTemplateContentToBuffer(content []byte) (*bytes.Buffer, error) {
	return r.Context.RenderTemplateToBuffer(string(content), r.Data)
}
