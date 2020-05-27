package state

import (
	"fmt"
	"reflect"

	"github.com/roboll/helmfile/pkg/tmpl"
	"gopkg.in/yaml.v2"
)

func (st *HelmState) Values() (map[string]interface{}, error) {
	return st.Env.GetMergedValues()
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
		Namespace:   st.OverrideNamespace,
		Values:      st.mustLoadVals(),
	}
}

func getBoolRefFromStringTemplate(templateRef string) (*bool, error) {
	var result bool
	if err := yaml.Unmarshal([]byte(templateRef), &result); err != nil {
		return nil, fmt.Errorf("failed deserialising string %s: %v", templateRef, err)
	}
	return &result, nil
}

func updateBoolTemplatedValues(r *ReleaseSpec) error {

	if r.InstalledTemplate != nil {
		if installed, err := getBoolRefFromStringTemplate(*r.InstalledTemplate); err != nil {
			return fmt.Errorf("installedTemplate: %v", err)
		} else {
			r.InstalledTemplate = nil
			r.Installed = installed
		}
	}

	if r.WaitTemplate != nil {
		if wait, err := getBoolRefFromStringTemplate(*r.WaitTemplate); err != nil {
			return fmt.Errorf("waitTemplate: %v", err)
		} else {
			r.WaitTemplate = nil
			r.Wait = wait
		}
	}

	if r.TillerlessTemplate != nil {
		if tillerless, err := getBoolRefFromStringTemplate(*r.TillerlessTemplate); err != nil {
			return fmt.Errorf("tillerlessTemplate: %v", err)
		} else {
			r.TillerlessTemplate = nil
			r.Tillerless = tillerless
		}
	}

	if r.VerifyTemplate != nil {
		if verify, err := getBoolRefFromStringTemplate(*r.VerifyTemplate); err != nil {
			return fmt.Errorf("verifyTemplate: %v", err)
		} else {
			r.VerifyTemplate = nil
			r.Verify = verify
		}
	}

	return nil
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
				successFlag = true
				if err := updateBoolTemplatedValues(r); err != nil {
					return nil, fmt.Errorf("failed executing templates in release \"%s\".\"%s\": %v", st.FilePath, rt.Name, err)
				}
				st.Releases[i] = *r
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
