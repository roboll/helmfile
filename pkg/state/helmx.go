package state

import (
	"fmt"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/remote"
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

func fileExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsRegular()
}

func directoryExistsAt(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.Mode().IsDir()
}

type Chartify struct {
	Opts  *chartify.ChartifyOpts
	Chart string
	Clean func()
}

func (st *HelmState) PrepareChartify(helm helmexec.Interface, release *ReleaseSpec, workerIndex int) (*Chartify, func(), error) {
	chartify := &Chartify{
		Opts: &chartify.ChartifyOpts{
			WorkaroundOutputDirIssue:    true,
			EnableKustomizeAlphaPlugins: true,
			ChartVersion:                release.Version,
			Namespace:                   release.Namespace,
		},
	}

	var filesNeedCleaning []string

	clean := func() {
		st.removeFiles(filesNeedCleaning)
	}

	var shouldRun bool

	chart := release.Chart
	if release.Directory != "" && chart == "" {
		chart = release.Directory
	}

	_, err := remote.Parse(chart)
	if err != nil {
		if release.ForceGoGetter {
			return nil, clean, fmt.Errorf("Parsing url from directory of release %q failed due to error %q.\nContinuing the process assuming this is a regular Helm chart or a local directory.", release.Name, err.Error())
		}
	} else {
		r := remote.NewRemote(st.logger, st.basePath, st.readFile, directoryExistsAt, fileExistsAt)

		fetchedDir, err := r.Fetch(chart)
		if err != nil {
			return nil, clean, fmt.Errorf("fetching %q: %v", chart, err)
		}

		chart = fetchedDir

		filesNeedCleaning = append(filesNeedCleaning, fetchedDir)
	}

	chartify.Chart = chart

	dir := filepath.Join(st.basePath, chart)
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

		chartify.Opts.AdhocChartDependencies = append(chartify.Opts.AdhocChartDependencies, dep)

		shouldRun = true
	}

	jsonPatches := release.JSONPatches
	if len(jsonPatches) > 0 {
		generatedFiles, err := st.generateTemporaryValuesFiles(jsonPatches, release.MissingFileHandler)
		if err != nil {
			return nil, clean, err
		}

		filesNeedCleaning = append(filesNeedCleaning, generatedFiles...)

		for _, f := range generatedFiles {
			chartify.Opts.JsonPatches = append(chartify.Opts.JsonPatches, f)
		}

		shouldRun = true
	}

	strategicMergePatches := release.StrategicMergePatches
	if len(strategicMergePatches) > 0 {
		generatedFiles, err := st.generateTemporaryValuesFiles(strategicMergePatches, release.MissingFileHandler)
		if err != nil {
			return nil, clean, err
		}

		for _, f := range generatedFiles {
			chartify.Opts.StrategicMergePatches = append(chartify.Opts.StrategicMergePatches, f)
		}

		filesNeedCleaning = append(filesNeedCleaning, generatedFiles...)

		shouldRun = true
	}

	if shouldRun {
		generatedFiles, err := st.generateValuesFiles(helm, release, workerIndex)
		if err != nil {
			return nil, clean, err
		}

		filesNeedCleaning = append(filesNeedCleaning, generatedFiles...)

		chartify.Opts.ValuesFiles = generatedFiles

		return chartify, clean, nil
	}

	return nil, clean, nil
}
