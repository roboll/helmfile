package app

import (
	"bytes"
	"fmt"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/state"
	"github.com/roboll/helmfile/tmpl"
	"strings"
)

func prependLineNumbers(text string) string {
	buf := bytes.NewBufferString("")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		buf.WriteString(fmt.Sprintf("%2d: %s\n", i, line))
	}
	return buf.String()
}

func (r *desiredStateLoader) renderEnvironment(firstPassEnv environment.Environment, baseDir, filename string, content []byte) environment.Environment {
	tmplData := state.EnvironmentTemplateData{Environment: firstPassEnv, Namespace: r.namespace}
	firstPassRenderer := tmpl.NewFirstPassRenderer(baseDir, tmplData)

	// parse as much as we can, tolerate errors, this is a preparse
	yamlBuf, err := firstPassRenderer.RenderTemplateContentToBuffer(content)
	if err != nil && r.logger != nil {
		r.logger.Debugf("first-pass rendering input of \"%s\":\n%s", filename, prependLineNumbers(string(content)))
		if yamlBuf == nil { // we have a template syntax error, let the second parse report
			r.logger.Debugf("template syntax error: %v", err)
			return firstPassEnv
		}
	}
	c := r.underlying()
	c.Strict = false
	// create preliminary state, as we may have an environment. Tolerate errors.
	prestate, err := c.ParseAndLoad(yamlBuf.Bytes(), baseDir, filename, r.env, false, &firstPassEnv)
	if err != nil && r.logger != nil {
		switch err.(type) {
		case *state.StateLoadError:
			r.logger.Infof("could not deduce `environment:` block, configuring only .Environment.Name. error: %v", err)
		}
		r.logger.Debugf("error in first-pass rendering: result of \"%s\":\n%s", filename, prependLineNumbers(yamlBuf.String()))
	}

	if prestate != nil {
		firstPassEnv = prestate.Env
	}
	return firstPassEnv
}

func (r *desiredStateLoader) renderTemplatesToYaml(baseDir, filename string, content []byte, context ...environment.Environment) (*bytes.Buffer, error) {
	var env environment.Environment

	if len(context) > 0 {
		env = context[0]
	} else {
		env = environment.Environment{Name: r.env, Values: map[string]interface{}(nil)}
	}

	return r.twoPassRenderTemplateToYaml(env, baseDir, filename, content)
}

func (r *desiredStateLoader) twoPassRenderTemplateToYaml(initEnv environment.Environment, baseDir, filename string, content []byte) (*bytes.Buffer, error) {
	// try a first pass render. This will always succeed, but can produce a limited env
	if r.logger != nil {
		r.logger.Debugf("first-pass rendering input of \"%s\": %v", filename, initEnv)
	}

	firstPassEnv := r.renderEnvironment(initEnv, baseDir, filename, content)

	if r.logger != nil {
		r.logger.Debugf("first-pass rendering result of \"%s\": %v", filename, firstPassEnv)
	}

	tmplData := state.EnvironmentTemplateData{Environment: firstPassEnv, Namespace: r.namespace}
	secondPassRenderer := tmpl.NewFileRenderer(r.readFile, baseDir, tmplData)
	yamlBuf, err := secondPassRenderer.RenderTemplateContentToBuffer(content)
	if err != nil {
		if r.logger != nil {
			r.logger.Debugf("second-pass rendering failed, input of \"%s\":\n%s", filename, prependLineNumbers(string(content)))
		}
		return nil, err
	}
	if r.logger != nil {
		r.logger.Debugf("second-pass rendering result of \"%s\":\n%s", filename, prependLineNumbers(yamlBuf.String()))
	}
	return yamlBuf, nil
}
