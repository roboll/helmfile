package state

import (
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/variantdev/chartify"
	"os"
	"path/filepath"
	"strings"
)

type Dependency struct {
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	Alias   string `yaml:"alias"`
}

func (st *HelmState) appendHelmXFlags(flags []string, release *ReleaseSpec) ([]string, error) {
	for _, adopt := range release.Adopt {
		flags = append(flags, "--adopt", adopt)
	}

	return flags, nil
}

func (st *HelmState) PrepareChartify(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) (bool, *chartify.ChartifyOpts, error) {
	var opts chartify.ChartifyOpts

	opts.WorkaroundOutputDirIssue = true

	var shouldRun bool

	opts.EnableKustomizeAlphaPlugins = true

	opts.ChartVersion = release.Version

	dir := filepath.Join(st.basePath, release.Chart)
	if stat, _ := os.Stat(dir); stat != nil && stat.IsDir() {
		if exists, err := st.fileExists(filepath.Join(dir, "Chart.yaml")); err == nil && !exists {
			shouldRun = true
		}
	}

	for _, d := range release.Dependencies {
		var dep string

		if d.Alias != "" {
			dep += d.Alias + "="
		} else {
			a := strings.Split(d.Chart, "/")

			chart := a[len(a)-1]

			dep += chart + "="
		}

		dep += d.Chart

		if d.Version != "" {
			dep += ":" + d.Version
		}

		opts.AdhocChartDependencies = append(opts.AdhocChartDependencies, dep)

		shouldRun = true
	}

	jsonPatches := release.JSONPatches
	if len(jsonPatches) > 0 {
		generatedFiles, err := st.generateTemporaryValuesFiles(jsonPatches, release.MissingFileHandler)
		if err != nil {
			return false, nil, err
		}

		for _, f := range generatedFiles {
			opts.JsonPatches = append(opts.JsonPatches, f)
		}

		release.generatedValues = append(release.generatedValues, generatedFiles...)

		shouldRun = true
	}

	strategicMergePatches := release.StrategicMergePatches
	if len(strategicMergePatches) > 0 {
		generatedFiles, err := st.generateTemporaryValuesFiles(strategicMergePatches, release.MissingFileHandler)
		if err != nil {
			return false, nil, err
		}

		for _, f := range generatedFiles {
			opts.StrategicMergePatches = append(opts.StrategicMergePatches, f)
		}

		release.generatedValues = append(release.generatedValues, generatedFiles...)

		shouldRun = true
	}

	if shouldRun {
		generatedFiles, err := st.generateValuesFiles(helm, release, workerIndex)
		if err != nil {
			return false, nil, err
		}

		opts.ValuesFiles = generatedFiles
	}

	return shouldRun, &opts, nil
}
