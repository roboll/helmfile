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
	positiveLabels map[string]string
	negativeLabels map[string]string
}

// Match will match a release that has the same labels as the filter
func (l LabelFilter) Match(r ReleaseSpec) bool {
	if len(l.positiveLabels) > 0 {
		for k, v := range l.positiveLabels {
			if rVal, ok := r.Labels[k]; !ok {
				return false
			} else if rVal != v {
				return false
			}
		}
	}
	if len(l.negativeLabels) > 0 {
		for k, v := range l.negativeLabels {
			if rVal, ok := r.Labels[k]; !ok {
				return true
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
	lf.positiveLabels = map[string]string{}
	lf.negativeLabels = map[string]string{}
	var err error
	labels := strings.Split(l, ",")
	for _, label := range labels {
		if match, _ := regexp.MatchString("^[a-zA-Z0-9_-]+!=[a-zA-Z0-9_-]+$", label); match == true { // k!=v case
			kv := strings.Split(label, "!=")
			lf.negativeLabels[kv[0]] = kv[1]
		} else if match, _ := regexp.MatchString("^[a-zA-Z0-9_-]+=[a-zA-Z0-9_-]+$", label); match == true { // k=v case
			kv := strings.Split(label, "=")
			lf.positiveLabels[kv[0]] = kv[1]
		} else { // malformed case
			err = fmt.Errorf("Malformed label: %s. Expected label in form k=v or k!=v", label)
		}
	}
	return lf, err
}
