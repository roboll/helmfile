package app

import (
	"bytes"
	"fmt"
	"github.com/roboll/helmfile/environment"
	"github.com/roboll/helmfile/state"
	"github.com/roboll/helmfile/tmpl"
	"go.uber.org/zap"
	"path/filepath"
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

type twoPassRenderer struct {
	reader    func(string) ([]byte, error)
	env       string
	namespace string
	filename  string
	logger    *zap.SugaredLogger
	abs       func(string) (string, error)
}

func (r *twoPassRenderer) renderEnvironment(content []byte) environment.Environment {
	firstPassEnv := environment.Environment{Name: r.env, Values: map[string]interface{}(nil)}
	tmplData := state.EnvironmentTemplateData{Environment: firstPassEnv, Namespace: r.namespace}
	firstPassRenderer := tmpl.NewFirstPassRenderer(filepath.Dir(r.filename), tmplData)

	// parse as much as we can, tolerate errors, this is a preparse
	yamlBuf, err := firstPassRenderer.RenderTemplateContentToBuffer(content)
	if err != nil && r.logger != nil {
		r.logger.Debugf("first-pass rendering input of \"%s\":\n%s", r.filename, prependLineNumbers(string(content)))
		if yamlBuf == nil { // we have a template syntax error, let the second parse report
			r.logger.Debugf("template syntax error: %v", err)
			return firstPassEnv
		}
	}
	c := state.NewCreator(r.logger, r.reader, r.abs)
	c.Strict = false
	// create preliminary state, as we may have an environment. Tolerate errors.
	prestate, err := c.CreateFromYaml(yamlBuf.Bytes(), r.filename, r.env)
	if err != nil && r.logger != nil {
		switch err.(type) {
		case *state.StateLoadError:
			r.logger.Infof("could not deduce `environment:` block, configuring only .Environment.Name. error: %v", err)
		}
		r.logger.Debugf("error in first-pass rendering: result of \"%s\":\n%s", r.filename, prependLineNumbers(yamlBuf.String()))
	}
	if prestate != nil {
		firstPassEnv = prestate.Env
	}
	return firstPassEnv
}

func (r *twoPassRenderer) renderTemplate(content []byte) (*bytes.Buffer, error) {
	// try a first pass render. This will always succeed, but can produce a limited env
	firstPassEnv := r.renderEnvironment(content)

	tmplData := state.EnvironmentTemplateData{Environment: firstPassEnv, Namespace: r.namespace}
	secondPassRenderer := tmpl.NewFileRenderer(r.reader, filepath.Dir(r.filename), tmplData)
	yamlBuf, err := secondPassRenderer.RenderTemplateContentToBuffer(content)
	if err != nil {
		if r.logger != nil {
			r.logger.Debugf("second-pass rendering failed, input of \"%s\":\n%s", r.filename, prependLineNumbers(string(content)))
		}
		return nil, err
	}
	if r.logger != nil {
		r.logger.Debugf("second-pass rendering result of \"%s\":\n%s", r.filename, prependLineNumbers(yamlBuf.String()))
	}
	return yamlBuf, nil
}
