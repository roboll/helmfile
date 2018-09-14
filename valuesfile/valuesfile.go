package valuesfile

import (
	"fmt"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/tmpl"
	"strings"
)

type renderer struct {
	readFile         func(string) ([]byte, error)
	tmplFileRenderer tmpl.FileRenderer
}

func NewRenderer(readFile func(filename string) ([]byte, error), basePath string, env environment.Environment) *renderer {
	return &renderer{
		readFile:         readFile,
		tmplFileRenderer: tmpl.NewFileRenderer(readFile, basePath, env, ""),
	}
}

func (r *renderer) RenderToBytes(path string) ([]byte, error) {
	var yamlBytes []byte
	splits := strings.Split(path, ".")
	if len(splits) > 0 && splits[len(splits)-1] == "gotmpl" {
		yamlBuf, err := r.tmplFileRenderer.RenderTemplateFileToBuffer(path)
		if err != nil {
			return nil, fmt.Errorf("failed to render [%s], because of %v", path, err)
		}
		yamlBytes = yamlBuf.Bytes()
	} else {
		var err error
		yamlBytes, err = r.readFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to load [%s]: %v", path, err)
		}
	}
	return yamlBytes, nil
}
