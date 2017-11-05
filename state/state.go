package state

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/roboll/helmfile/helmexec"

	yaml "gopkg.in/yaml.v1"
	"path"
	"regexp"
)

type HelmState struct {
	BaseChartPath string
	Repositories  []RepositorySpec `yaml:"repositories"`
	Charts        []ChartSpec      `yaml:"charts"`
}

type RepositorySpec struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type ChartSpec struct {
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	Verify  bool   `yaml:"verify"`

	Name      string     `yaml:"name"`
	Namespace string     `yaml:"namespace"`
	Values    []string   `yaml:"values"`
	SetValues []SetValue `yaml:"set"`
	EnvValues []SetValue `yaml:"env"`
}

type SetValue struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

func ReadFromFile(file string) (*HelmState, error) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var state HelmState

	state.BaseChartPath, _ = filepath.Abs(path.Dir(file))
	if err := yaml.Unmarshal(content, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (state *HelmState) SyncRepos(helm helmexec.Interface) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, repo := range state.Repositories {
		wg.Add(1)
		go func(wg *sync.WaitGroup, name, url string) {
			if err := helm.AddRepo(name, url); err != nil {
				errs = append(errs, err)
			}
			wg.Done()
		}(&wg, repo.Name, repo.URL)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	if err := helm.UpdateRepo(); err != nil {
		return []error{err}
	}
	return nil
}

func (state *HelmState) SyncCharts(helm helmexec.Interface, additonalValues []string) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, chart := range state.Charts {
		wg.Add(1)
		go func(wg *sync.WaitGroup, chart ChartSpec) {
			flags, flagsErr := flagsForChart(state.BaseChartPath, &chart)
			if flagsErr != nil {
				errs = append(errs, flagsErr)
			}
			for _, value := range additonalValues {
				valfile, err := filepath.Abs(value)
				if err != nil {
					errs = append(errs, err)
				}
				flags = append(flags, "--values", valfile)
			}
			if len(errs) == 0 {
				if err := helm.SyncChart(chart.Name, normalizeChart(state.BaseChartPath, chart.Chart), flags...); err != nil {
					errs = append(errs, err)
				}
			}
			wg.Done()
		}(&wg, chart)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (state *HelmState) DiffCharts(helm helmexec.Interface, additonalValues []string) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, chart := range state.Charts {
		wg.Add(1)
		go func(wg *sync.WaitGroup, chart ChartSpec) {
			// Plugin command doesn't support explicit namespace
			chart.Namespace = ""
			flags, flagsErr := flagsForChart(state.BaseChartPath, &chart)
			if flagsErr != nil {
				errs = append(errs, flagsErr)
			}
			for _, value := range additonalValues {
				valfile, err := filepath.Abs(value)
				if err != nil {
					errs = append(errs, err)
				}
				flags = append(flags, "--values", valfile)
			}
			if len(errs) == 0 {
				if err := helm.DiffChart(chart.Name, normalizeChart(state.BaseChartPath, chart.Chart), flags...); err != nil {
					errs = append(errs, err)
				}
			}
			wg.Done()
		}(&wg, chart)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (state *HelmState) DeleteCharts(helm helmexec.Interface) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, chart := range state.Charts {
		wg.Add(1)
		go func(wg *sync.WaitGroup, chart ChartSpec) {
			if err := helm.DeleteChart(chart.Name); err != nil {
				errs = append(errs, err)
			}
			wg.Done()
		}(&wg, chart)
	}
	wg.Wait()

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// normalizeChart allows for the distinction between a file path reference and repository references.
// - Any single (or double character) followed by a `/` will be considered a local file reference and
// 	 be constructed relative to the `base path`.
// - Everything else is assumed to be an absolute path or an actual <repository>/<chart> reference.
func normalizeChart(basePath, chart string) (string) {
	regex, _ := regexp.Compile("^[.]?./")
	if !regex.MatchString(chart) {
		return chart
	}
	return filepath.Join(basePath, chart)
}

func flagsForChart(basePath string, chart *ChartSpec) ([]string, error) {
	flags := []string{}
	if chart.Version != "" {
		flags = append(flags, "--version", chart.Version)
	}
	if chart.Verify {
		flags = append(flags, "--verify")
	}
	if chart.Namespace != "" {
		flags = append(flags, "--namespace", chart.Namespace)
	}
	for _, value := range chart.Values {
		valfile := filepath.Join(basePath, value)
		flags = append(flags, "--values", valfile)
	}
	if len(chart.SetValues) > 0 {
		val := []string{}
		for _, set := range chart.SetValues {
			val = append(val, fmt.Sprintf("%s=%s", set.Name, set.Value))
		}
		flags = append(flags, "--set", strings.Join(val, ","))
	}
	if len(chart.EnvValues) > 0 {
		val := []string{}
		envValErrs := []string{}
		for _, set := range chart.EnvValues {
			value, isSet := os.LookupEnv(set.Value)
			if isSet {
				val = append(val, fmt.Sprintf("%s=%s", set.Name, value))
			} else {
				errMsg := fmt.Sprintf("\t%s", set.Value)
				envValErrs = append(envValErrs, errMsg)
			}
		}
		if len(envValErrs) != 0 {
			joinedEnvVals := strings.Join(envValErrs, "\n")
			errMsg := fmt.Sprintf("Environment Variables not found. Please make sure they are set and try again:\n%s", joinedEnvVals)
			return nil, errors.New(errMsg)
		}
		flags = append(flags, "--set", strings.Join(val, ","))
	}
	return flags, nil
}
