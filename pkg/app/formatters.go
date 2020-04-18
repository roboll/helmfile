package app

import (
	"encoding/json"
	"fmt"

	"github.com/gosuri/uitable"
)

func FormatAsTable(releases []*HelmRelease) {
	table := uitable.New()
	table.AddRow("NAME", "NAMESPACE", "ENABLED", "LABELS")

	for _, r := range releases {
		table.AddRow(r.Name, r.Namespace, fmt.Sprintf("%t", r.Enabled), r.Labels)
	}

	fmt.Println(table.String())
}

func FormatAsJson(releases []*HelmRelease) {
	output, err := json.Marshal(releases)

	if err != nil {
		fmt.Errorf("error generating json: %v", err)
	}

	fmt.Println(string(output))
}
