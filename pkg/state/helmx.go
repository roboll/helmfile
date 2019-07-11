package state

type Dependency struct {
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	Alias   string `yaml:"alias"`
}

func (st *HelmState) appendHelmXFlags(flags []string, release *ReleaseSpec) ([]string, error) {
	for _, d := range release.Dependencies {
		var dep string
		if d.Alias != "" {
			dep += d.Alias + "="
		}
		dep += d.Chart
		if d.Version != "" {
			dep += ":" + d.Version
		}
		flags = append(flags, "--dependency", dep)
	}

	for _, adopt := range release.Adopt {
		flags = append(flags, "--adopt", adopt)
	}

	jsonPatches := release.JSONPatches
	if len(jsonPatches) > 0 {
		generatedFiles, err := st.generateTemporaryValuesFiles(jsonPatches, release.MissingFileHandler)
		if err != nil {
			return nil, err
		}

		for _, f := range generatedFiles {
			flags = append(flags, "--json-patch", f)
		}

		release.generatedValues = append(release.generatedValues, generatedFiles...)
	}

	strategicMergePatches := release.StrategicMergePatches
	if len(strategicMergePatches) > 0 {
		generatedFiles, err := st.generateTemporaryValuesFiles(strategicMergePatches, release.MissingFileHandler)
		if err != nil {
			return nil, err
		}

		for _, f := range generatedFiles {
			flags = append(flags, "--strategic-merge-patch", f)
		}

		release.generatedValues = append(release.generatedValues, generatedFiles...)
	}

	return flags, nil
}
