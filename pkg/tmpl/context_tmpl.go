package tmpl

import (
	"bytes"
	"github.com/Masterminds/sprig"
	"text/template"
)

func (c *Context) newTemplate() *template.Template {
	funcMap := sprig.TxtFuncMap()
	for name, f := range c.createFuncMap() {
		funcMap[name] = f
	}
	tmpl := template.New("stringTemplate").Funcs(funcMap)
	if c.preRender {
		tmpl = tmpl.Option("missingkey=zero")
	} else {
		tmpl = tmpl.Option("missingkey=error")
	}
	return tmpl
}

func (c *Context) RenderTemplateToBuffer(s string, data ...interface{}) (*bytes.Buffer, error) {
	var t, parseErr = c.newTemplate().Parse(s)
	if parseErr != nil {
		return nil, parseErr
	}

	var tplString bytes.Buffer
	var d interface{}
	if len(data) > 0 {
		d = data[0]
	}
	var execErr = t.Execute(&tplString, d)

	if execErr != nil {
		return &tplString, execErr
	}

	return &tplString, nil
}
