package tmpl

import (
	"bytes"
	"github.com/Masterminds/sprig"
	"text/template"
)

func (c *Context) stringTemplate() *template.Template {
	funcMap := sprig.TxtFuncMap()
	for name, f := range c.createFuncMap() {
		funcMap[name] = f
	}
	return template.New("stringTemplate").Funcs(funcMap)
}

func (c *Context) RenderTemplateToBuffer(s string) (*bytes.Buffer, error) {
	var t, parseErr = c.stringTemplate().Parse(s)
	if parseErr != nil {
		return nil, parseErr
	}

	var tplString bytes.Buffer
	var execErr = t.Execute(&tplString, nil)

	if execErr != nil {
		return nil, execErr
	}

	return &tplString, nil
}
