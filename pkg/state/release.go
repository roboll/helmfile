package state

import (
	"fmt"

	"github.com/roboll/helmfile/pkg/tmpl"
	"gopkg.in/yaml.v2"
)

func (r ReleaseSpec) ExecuteTemplateExpressions(renderer *tmpl.FileRenderer) (*ReleaseSpec, error) {
	var result *ReleaseSpec
	var err error

	result, err = r.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed executing template expressions in release \"%s\": %v", r.Name, err)
	}

	{
		ts := result.Name
		result.Name, err = renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".name = \"%s\": %v", r.Name, ts, err)
		}
	}

	{
		ts := result.Chart
		result.Chart, err = renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".chart = \"%s\": %v", r.Name, ts, err)
		}
	}

	{
		ts := result.Namespace
		result.Namespace, err = renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".namespace = \"%s\": %v", r.Name, ts, err)
		}
	}

	{
		ts := result.Version
		result.Version, err = renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".version = \"%s\": %v", r.Name, ts, err)
		}
	}

	if result.WaitTemplate != nil {
		ts := *result.WaitTemplate
		resultTmpl, err := renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".version = \"%s\": %v", r.Name, ts, err)
		}
		result.WaitTemplate = &resultTmpl
	}

	if result.InstalledTemplate != nil {
		ts := *result.InstalledTemplate
		resultTmpl, err := renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".version = \"%s\": %v", r.Name, ts, err)
		}
		result.InstalledTemplate = &resultTmpl
	}

	if result.TillerlessTemplate != nil {
		ts := *result.TillerlessTemplate
		resultTmpl, err := renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".version = \"%s\": %v", r.Name, ts, err)
		}
		result.TillerlessTemplate = &resultTmpl
	}

	if result.VerifyTemplate != nil {
		ts := *result.VerifyTemplate
		resultTmpl, err := renderer.RenderTemplateContentToString([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".version = \"%s\": %v", r.Name, ts, err)
		}
		result.VerifyTemplate = &resultTmpl
	}

	for key, val := range result.Labels {
		ts := val
		s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".labels[%s] = \"%s\": %v", r.Name, key, ts, err)
		}
		result.Labels[key] = s.String()
	}

	for i, t := range result.Values {
		switch ts := t.(type) {
		case string:
			s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
			if err != nil {
				return nil, fmt.Errorf("failed executing template expressions in release \"%s\".values[%d] = \"%s\": %v", r.Name, i, ts, err)
			}
			result.Values[i] = s.String()
		}
	}

	for i, ts := range result.Secrets {
		s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
		if err != nil {
			return nil, fmt.Errorf("failed executing template expressions in release \"%s\".secrets[%d] = \"%s\": %v", r.Name, i, ts, err)
		}
		result.Secrets[i] = s.String()
	}

	for i, val := range result.SetValues {
		{
			// name
			ts := val.Name
			s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
			if err != nil {
				return nil, fmt.Errorf("failed executing template expressions in release \"%s\".set[%d].name = \"%s\": %v", r.Name, i, ts, err)
			}
			result.SetValues[i].Name = s.String()
		}
		{
			// value
			ts := val.Value
			s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
			if err != nil {
				return nil, fmt.Errorf("failed executing template expressions in release \"%s\".set[%d].value = \"%s\": %v", r.Name, i, ts, err)
			}
			result.SetValues[i].Value = s.String()
		}
		{
			// file
			ts := val.File
			s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
			if err != nil {
				return nil, fmt.Errorf("failed executing template expressions in release \"%s\".set[%d].file = \"%s\": %v", r.Name, i, ts, err)
			}
			result.SetValues[i].File = s.String()
		}
		for j, ts := range val.Values {
			// values
			s, err := renderer.RenderTemplateContentToBuffer([]byte(ts))
			if err != nil {
				return nil, fmt.Errorf("failed executing template expressions in release \"%s\".set[%d].values[%d] = \"%s\": %v", r.Name, i, j, ts, err)
			}
			result.SetValues[i].Values[j] = s.String()
		}
	}

	return result, nil
}

func (r ReleaseSpec) Clone() (*ReleaseSpec, error) {
	serialized, err := yaml.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed cloning release \"%s\": %v", r.Name, err)
	}

	var deserialized ReleaseSpec

	if err := yaml.Unmarshal(serialized, &deserialized); err != nil {
		return nil, fmt.Errorf("failed cloning release \"%s\": %v", r.Name, err)
	}

	return &deserialized, nil
}

func (r ReleaseSpec) Desired() bool {
	return r.Installed == nil || *r.Installed
}
