package state

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/roboll/helmfile/helmexec"

	yaml "gopkg.in/yaml.v1"
)

type HelmState struct {
	Repositories []RepositorySpec `yaml:"repositories"`
	Charts       []ChartSpec      `yaml:"charts"`
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

func (state *HelmState) SyncCharts(helm helmexec.Interface) []error {
	var wg sync.WaitGroup
	errs := []error{}

	for _, chart := range state.Charts {
		wg.Add(1)
		go func(wg *sync.WaitGroup, chart ChartSpec) {
			flags, err := flagsForChart(&chart)
			if err != nil {
				errs = append(errs, err)
			} else {
				if err := helm.SyncChart(chart.Name, chart.Chart, flags...); err != nil {
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

func flagsForChart(chart *ChartSpec) ([]string, error) {
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
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		valfile := filepath.Join(wd, value)
		flags = append(flags, "--values", valfile)
	}
	if len(chart.SetValues) > 0 {
		val := []string{}
		for _, set := range chart.SetValues {
			val = append(val, fmt.Sprintf("%s=%s", set.Name, set.Value))
		}
		flags = append(flags, "--set", strings.Join(val, ","))
	}
	return flags, nil
}
