package state

import(
	"github.com/variantdev/chartify"
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

func (st *HelmState) PrepareChartify(release *ReleaseSpec) (bool, *chartify.ChartifyOpts) {
	var opts chartify.ChartifyOpts

	var shouldRun bool

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
			return false, nil
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
			return false, nil
		}

		for _, f := range generatedFiles {
			opts.StrategicMergePatches = append(opts.StrategicMergePatches, f)
		}

		release.generatedValues = append(release.generatedValues, generatedFiles...)

		shouldRun = true
	}

	return shouldRun, &opts
}
