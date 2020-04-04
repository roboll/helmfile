package state

import (
	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"io/ioutil"
	"path/filepath"
	"testing"
)

func newLoader() *EnvironmentValuesLoader {
	log, err := zap.NewDevelopment(zap.AddStacktrace(zap.DebugLevel))
	if err != nil {
		panic(err)
	}

	sugar := log.Sugar()

	storage := &Storage{
		FilePath: "./helmfile.yaml",
		basePath: ".",
		glob:     filepath.Glob,
		logger:   sugar,
	}

	return NewEnvironmentValuesLoader(storage, ioutil.ReadFile, sugar)
}

// See https://github.com/roboll/helmfile/pull/1169
func TestEnvValsLoad_SingleValuesFile(t *testing.T) {
	l := newLoader()

	actual, err := l.LoadEnvironmentValues(nil, []interface{}{"testdata/values.5.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"affinity": map[string]interface{}{},
	}

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf(diff)
	}
}

// See https://github.com/roboll/helmfile/issues/1150
func TestEnvValsLoad_OverwriteNilValue_Issue1150(t *testing.T) {
	l := newLoader()

	actual, err := l.LoadEnvironmentValues(nil, []interface{}{"testdata/values.1.yaml", "testdata/values.2.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"components": map[string]interface{}{
			"etcd-operator": map[string]interface{}{
				"version": "0.10.3",
			},
		},
	}

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf(diff)
	}
}

// See https://github.com/roboll/helmfile/issues/1154
func TestEnvValsLoad_OverwriteWithNilValue_Issue1154(t *testing.T) {
	l := newLoader()

	actual, err := l.LoadEnvironmentValues(nil, []interface{}{"testdata/values.3.yaml", "testdata/values.4.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"components": map[string]interface{}{
			"etcd-operator": map[string]interface{}{
				"version": "0.10.3",
			},
			"prometheus": nil,
		},
	}

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf(diff)
	}
}

// See https://github.com/roboll/helmfile/issues/1168
func TestEnvValsLoad_OverwriteEmptyValue_Issue1168(t *testing.T) {
	l := newLoader()

	actual, err := l.LoadEnvironmentValues(nil, []interface{}{"testdata/issues/1168/addons.yaml", "testdata/issues/1168/addons2.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"addons": map[string]interface{}{
			"mychart": map[string]interface{}{
				"skip":      false,
				"name":      "mychart",
				"namespace": "kube-system",
				"chart":     "stable/mychart",
				"version":   "1.0.0",
			},
		},
	}

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf(diff)
	}
}
