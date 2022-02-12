package app

import (
	"encoding/json"
	"fmt"

	"github.com/gosuri/uitable"
)

func FormatAsTable(releases []*HelmRelease) error {
	table := uitable.New()
	table.AddRow("NAME", "NAMESPACE", "ENABLED", "INSTALLED", "LABELS", "CHART", "VERSION")

	for _, r := range releases {
		table.AddRow(r.Name, r.Namespace, fmt.Sprintf("%t", r.Enabled), fmt.Sprintf("%t", r.Installed), r.Labels, r.Chart, r.Version)
	}

	fmt.Println(table.String())

	return nil
}

func FormatAsJson(releases []*HelmRelease) error {
	output, err := json.Marshal(releases)

	if err != nil {
		return fmt.Errorf("error generating json: %v", err)
	}

	fmt.Println(string(output))

	return nil
}
