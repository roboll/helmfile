package state

import (
	"fmt"
	"github.com/roboll/helmfile/tmpl"
)

func (st *HelmState) envTemplateData() EnvironmentTemplateData {
	return EnvironmentTemplateData{
		st.Env,
		st.Namespace,
	}
}

func (st *HelmState) ExecuteTemplates() (*HelmState, error) {
	r := *st

	for i, rt := range st.Releases {
		tmplData := ReleaseTemplateData{
			Environment: st.Env,
			Release:     rt,
		}
		renderer := tmpl.NewFileRenderer(st.readFile, st.basePath, tmplData)
		r, err := rt.ExecuteTemplateExpressions(renderer)
		if err != nil {
			return nil, fmt.Errorf("failed executing templates in release \"%s\".\"%s\": %v", st.FilePath, rt.Name, err)
		}
		st.Releases[i] = *r
	}

	return &r, nil
}
