package state

import (
	"fmt"
	"regexp"
	"strings"
)

// ReleaseFilter is used to determine if a given release should be used during helmfile execution
type ReleaseFilter interface {
	// Match returns true if the ReleaseSpec matches the Filter
	Match(r ReleaseSpec) bool
}

// LabelFilter matches a release with the given positive lables. Negative labels
// invert the match for cases such as tier!=backend
type LabelFilter struct {
	positiveLabels [][]string
	negativeLabels [][]string
}

// Match will match a release that has the same labels as the filter
func (l LabelFilter) Match(r ReleaseSpec) bool {
	if len(l.positiveLabels) > 0 {
		for _, element := range l.positiveLabels {
			k := element[0]
			v := element[1]
			if rVal, ok := r.Labels[k]; !ok {
				return false
			} else if rVal != v {
				return false
			}
		}
	}

	if len(l.negativeLabels) > 0 {
		for _, element := range l.negativeLabels {
			k := element[0]
			v := element[1]
			if rVal, ok := r.Labels[k]; !ok {

			} else if rVal == v {
				return false
			}
		}
	}
	return true
}

// ParseLabels takes a label in the form foo=bar,baz!=bat and returns a LabelFilter that will match the labels
func ParseLabels(l string) (LabelFilter, error) {
	lf := LabelFilter{}
	lf.positiveLabels = [][]string{}
	lf.negativeLabels = [][]string{}
	var err error
	labels := strings.Split(l, ",")
	for _, label := range labels {
		if match, _ := regexp.MatchString("^[a-zA-Z0-9_-]+!=[a-zA-Z0-9_-]+$", label); match == true { // k!=v case
			kv := strings.Split(label, "!=")
			lf.negativeLabels = append(lf.negativeLabels, kv)
		} else if match, _ := regexp.MatchString("^[a-zA-Z0-9_-]+=[a-zA-Z0-9_-]+$", label); match == true { // k=v case
			kv := strings.Split(label, "=")
			lf.positiveLabels = append(lf.positiveLabels, kv)
		} else { // malformed case
			return lf, fmt.Errorf("Malformed label: %s. Expected label in form k=v or k!=v", label)
		}
	}
	return lf, err
}
