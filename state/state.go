package state

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/roboll/helmfile/helmexec"

	yaml "gopkg.in/yaml.v1"
	"path"
	"regexp"
	"bytes"
)

type HelmState struct {
	BaseChartPath string
	Context       string           `yaml:"context"`
	Repositories  []RepositorySpec `yaml:"repositories"`
	Charts        []ChartSpec      `yaml:"charts"`
}

type RepositorySpec struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type ChartSpec struct {
	Chart     string `yaml:"chart"`
	Version   string `yaml:"version"`
	Verify    bool   `yaml:"verify"`

	Name      string     `yaml:"name"`
	Namespace string     `yaml:"namespace"`
	Values    []string   `yaml:"values"`
	SetValues []SetValue `yaml:"set"`

	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
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

var /* const */
	stringTemplateFuncMap = template.FuncMap{
		"env": getEnvVar,
	}

var /* const */
	stringTemplate = template.New("stringTemplate").Funcs(stringTemplateFuncMap)

func getEnvVar(envVarName string) (string, error) {
	envVarValue, isSet := os.LookupEnv(envVarName)

	if !isSet {
		errMsg := fmt.Sprintf("Environment Variable '%s' is not set. Please make sure it is set and try again.", envVarName)
		return "", errors.New(errMsg)
	}

	return envVarValue, nil
}

func renderTemplateString(s string) (string, error) {
	var t, parseErr = stringTemplate.Parse(s)
	if parseErr != nil {
		return "", parseErr
	}

	var tplString bytes.Buffer
	var execErr = t.Execute(&tplString, nil)

	if execErr != nil {
		return "", execErr
	}

	return tplString.String(), nil
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

func (state *HelmState) SyncCharts(helm helmexec.Interface, additonalValues []string, workerLimit int) []error {
	errs := []error{}
	jobQueue := make(chan ChartSpec)
	doneQueue := make(chan bool)
	errQueue := make(chan error)

	if workerLimit < 1 {
		workerLimit = len(state.Charts)
	}

	for w := 1; w <= workerLimit; w++ {
		go func() {
			for chart := range jobQueue {

				flags, flagsErr := flagsForChart(state.BaseChartPath, &chart)
				if flagsErr != nil {
					errQueue <- flagsErr
					doneQueue <- true
					continue
				}

				haveValueErr := false
				for _, value := range additonalValues {
					valfile, err := filepath.Abs(value)
					if err != nil {
						errQueue <- err
						haveValueErr = true
					}
					flags = append(flags, "--values", valfile)
				}

				if haveValueErr {
					doneQueue <- true
					continue
				}

				if err := helm.SyncChart(chart.Name, normalizeChart(state.BaseChartPath, chart.Chart), flags...); err != nil {
					errQueue <- err
				}
				doneQueue <- true
			}
		}()
	}

	go func() {
		for _, chart := range state.Charts {
			jobQueue <- chart
		}
		close(jobQueue)
	}()

	for i := 0; i < len(state.Charts); {
		select {
		case err := <-errQueue:
			errs = append(errs, err)
		case <-doneQueue:
			i++
		}
	}

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
func normalizeChart(basePath, chart string) string {
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
		valfileRendered, err := renderTemplateString(valfile)
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--values", valfileRendered)
	}
	if len(chart.SetValues) > 0 {
		val := []string{}
		for _, set := range chart.SetValues {
			renderedValue, err := renderTemplateString(set.Value)
			if err != nil {
				return nil, err
			}
			val = append(val, fmt.Sprintf("%s=%s", set.Name, renderedValue))
		}
		flags = append(flags, "--set", strings.Join(val, ","))
	}

	/***********
	 * START 'env' section for backwards compatibility
	 ***********/
	// The 'env' section is not really necessary any longer, as 'set' would now provide the same functionality
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
	/**************
	 * END 'env' section for backwards compatibility
	 **************/

	return flags, nil
}
