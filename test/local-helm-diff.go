package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/databus23/helm-diff/diff"
	"github.com/databus23/helm-diff/manifest"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/sets"
)

var (
	rootCmd = &cobra.Command{
		Use:   "local-helm-diff dir-with-yamls/ dir-with-yamls/",
		Short: "Print any diff between the given directories",
		Long: `Similar to the 'diff' command, but file contents
are compared after being parsed as a set of Kubernetes manifests.`,
		Args: cobra.ExactArgs(2),
		Run:  run,
	}
)

func run(cmd *cobra.Command, args []string) {
	left := args[0]
	right := args[1]
	leftYamls, err := globYamlFilenames(left)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	rightYamls, err := globYamlFilenames(right)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	exitCode := 0

	onlyInLeft := leftYamls.Difference(rightYamls)
	if onlyInLeft.Len() > 0 {
		exitCode = 1
		for _, f := range onlyInLeft.List() {
			fmt.Fprintf(os.Stderr, "Only in %s: %s\n", left, f)
		}
	}

	onlyInRight := rightYamls.Difference(leftYamls)
	if onlyInRight.Len() > 0 {
		exitCode = 1
		for _, f := range onlyInRight.List() {
			fmt.Fprintf(os.Stderr, "Only in %s: %s\n", right, f)
		}
	}

	inBoth := leftYamls.Intersection(rightYamls)
	for _, f := range inBoth.List() {
		leftManifest, err := readManifest(filepath.Join(left, f))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		rightManifest, err := readManifest(filepath.Join(right, f))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		hasChanges := diff.Manifests(leftManifest, rightManifest, nil, false, 5, os.Stderr)
		if hasChanges {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func globYamlFilenames(dir string) (sets.String, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	set := sets.String{}
	for _, f := range matches {
		set.Insert(filepath.Base(f))
	}
	return set, nil
}

func readManifest(path string) (map[string]*manifest.MappingResult, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return manifest.Parse(string(bytes), "default"), nil
}

func main() {
	rootCmd.Execute()
}
