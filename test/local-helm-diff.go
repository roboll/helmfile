package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-test/deep"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/sets"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type diffSource string

const (
	diffSourceLeft  diffSource = "left"
	diffSourceRight diffSource = "right"
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
		leftPath := filepath.Join(left, f)
		leftNodes, err := readManifest(leftPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		rightPath := filepath.Join(right, f)
		rightNodes, err := readManifest(rightPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		ps := pairs{}
		for _, node := range leftNodes {
			if err := ps.add(node, diffSourceLeft); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		}
		for _, node := range rightNodes {
			if err := ps.add(node, diffSourceRight); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		}
		for _, p := range ps.list {
			switch {
			case p.left != nil && p.right == nil:
				fmt.Fprintf(os.Stderr, "Only in %s: %s\n", leftPath, nodeToString(p.left))
				exitCode = 1
			case p.left == nil && p.right != nil:
				fmt.Fprintf(os.Stderr, "Only in %s: %s\n", rightPath, nodeToString(p.right))
				exitCode = 1
			default:
				diff := deep.Equal(p.left, p.right)
				if diff != nil {
					id := nodeToString(p.left)
					fmt.Fprintf(os.Stderr, "--- %s %s", id, leftPath)
					fmt.Fprintf(os.Stderr, "+++ %s %s", id, rightPath)
					for _, d := range diff {
						fmt.Fprintf(os.Stderr, d)
					}
					exitCode = 1
				}
			}
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

func readManifest(path string) ([]*yaml.RNode, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rr := &kio.ByteReader{
		DisableUnwrapping:     true,
		Reader:                f,
		OmitReaderAnnotations: true,
	}
	nodes, err := rr.Read()
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		node := nodes[i].YNode()
		node.HeadComment = ""
	}
	return nodes, nil
}

func nodeToString(node *yaml.RNode) string {
	meta, err := node.GetMeta()
	if err != nil {
		return fmt.Sprintf("%v", node)
	}
	ns := meta.Namespace
	if ns == "" {
		ns = "~X"
	}
	nm := meta.Name
	if nm == "" {
		nm = "~N"
	}
	gv := meta.APIVersion
	if gv == "" {
		gv = "~G_~V"
	}
	k := meta.Kind
	if k == "" {
		k = "~K"
	}
	gvk := strings.Join([]string{gv, k}, "_")
	return strings.Join([]string{gvk, ns, nm}, "|")

}

// lifted from kustomize/kyaml/kio/filters/merge3.go
type pairs struct {
	list []*pair
}

func (ps *pairs) isSameResource(meta1, meta2 yaml.ResourceMeta) bool {
	if meta1.Name != meta2.Name {
		return false
	}
	if meta1.Namespace != meta2.Namespace {
		return false
	}
	if meta1.APIVersion != meta2.APIVersion {
		return false
	}
	if meta1.Kind != meta2.Kind {
		return false
	}
	return true
}

func (ps *pairs) add(node *yaml.RNode, source diffSource) error {
	nodeMeta, err := node.GetMeta()
	if err != nil {
		return err
	}
	for i := range ps.list {
		p := ps.list[i]
		if ps.isSameResource(p.meta, nodeMeta) {
			return p.add(node, source)
		}
	}
	p := &pair{meta: nodeMeta}
	if err := p.add(node, source); err != nil {
		return err
	}
	ps.list = append(ps.list, p)
	return nil
}

type pair struct {
	meta  yaml.ResourceMeta
	left  *yaml.RNode
	right *yaml.RNode
}

func (p *pair) add(node *yaml.RNode, source diffSource) error {
	switch source {
	case diffSourceLeft:
		if p.left != nil {
			return fmt.Errorf("left source already specified")
		}
		p.left = node
	case diffSourceRight:
		if p.right != nil {
			return fmt.Errorf("right source already specified")
		}
		p.right = node
	default:
		return fmt.Errorf("unknown diff source value: %s", source)
	}
	return nil

}

func main() {
	rootCmd.Execute()
}
