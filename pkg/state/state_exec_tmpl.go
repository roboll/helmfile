package state

import (
	"fmt"
	"reflect"

	"github.com/imdario/mergo"
	"github.com/roboll/helmfile/pkg/maputil"
	"github.com/roboll/helmfile/pkg/tmpl"
)

func (st *HelmState) Values() (map[string]interface{}, error) {
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
		successFlag := false
		for it, prev := 0, &rt; it < 6; it++ {
			tmplData := releaseTemplateData{
				Environment: st.Env,
				Release:     *prev,
				Values:      vals,
			}
			renderer := tmpl.NewFileRenderer(st.readFile, st.basePath, tmplData)
			r, err := rt.ExecuteTemplateExpressions(renderer)
			if err != nil {
				return nil, fmt.Errorf("failed executing templates in release \"%s\".\"%s\": %v", st.FilePath, rt.Name, err)
			}
			if reflect.DeepEqual(prev, r) {
				st.Releases[i] = *r
				successFlag = true
				break
			}
			prev = r
		}
		if !successFlag {
			return nil, fmt.Errorf("failed executing templates in release \"%s\".\"%s\": %s", st.FilePath, rt.Name,
				"recursive references can't be resolved")
		}
	}

	return &r, nil
}
