package state

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/pkg/maputil"
	"github.com/roboll/helmfile/pkg/tmpl"
)

func (st *HelmState) Values() (map[string]interface{}, error) {
	st.valsMutex.Lock()
	defer st.valsMutex.Unlock()
	if st.vals != nil {
		return st.vals, nil
	}

	vals := map[string]interface{}{}

	if err := mergo.Merge(&vals, st.Env.Defaults, mergo.WithOverride); err != nil {
		return nil, err
	}
	if err := mergo.Merge(&vals, st.Env.Values, mergo.WithOverride); err != nil {
		return nil, err
	}

	vals, err := maputil.CastKeysToStrings(vals)
	if err != nil {
		return nil, err
	}

	st.vals = vals

	return vals, nil
}

func (st *HelmState) mustLoadVals() map[string]interface{} {
	vals, err := st.Values()
	if err != nil {
		panic(err)
	}
	return vals
}

func (st *HelmState) valuesFileTemplateData() EnvironmentTemplateData {
	return EnvironmentTemplateData{
		Environment: st.Env,
		Namespace:   st.Namespace,
		Values:      st.mustLoadVals(),
	}
}

func (st *HelmState) ExecuteTemplates() (*HelmState, error) {
	r := *st

	vals, err := st.Values()
	if err != nil {
		return nil, err
	}

	for i, rt := range st.Releases {
		tmplData := releaseTemplateData{
			Environment: st.Env,
			Release:     rt,
			Values:      vals,
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
