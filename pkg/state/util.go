package state

import (
	"regexp"
	"strings"
)

func isLocalChart(chart string) bool {
	regex, _ := regexp.Compile("^[.]?./")
	matched := regex.MatchString(chart)
	if matched {
		return true
	}

	uriLike := strings.Index(chart, "://") > -1
	if uriLike {
		return false
	}

	return chart == "" ||
		chart[0] == '/' ||
		strings.Index(chart, "/") == -1 ||
		len(strings.Split(chart, "/")) != 2
}

func resolveRemoteChart(repoAndChart string) (string, string, bool) {
	if isLocalChart(repoAndChart) {
		return "", "", false
	}

	parts := strings.Split(repoAndChart, "/")
	if len(parts) != 2 {
		return "", "", false
	}

	repo := parts[0]
	chart := parts[1]

	return repo, chart, true
}
