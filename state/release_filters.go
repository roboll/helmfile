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

// TagFilter matches a release with the given positive tags. Negative tags
// invert the match for cases such as tier!=backend
type TagFilter struct {
	positiveTags map[string]string
	negativeTags map[string]string
}

// Match will match a release that has the same tags as the filter
func (t TagFilter) Match(r ReleaseSpec) bool {
	if len(t.positiveTags) > 0 {
		for k, v := range t.positiveTags {
			if rVal, ok := r.Tags[k]; !ok {
				return false
			} else if rVal != v {
				return false
			}
		}
	}
	if len(t.negativeTags) > 0 {
		for k, v := range t.negativeTags {
			if rVal, ok := r.Tags[k]; !ok {
				return true
			} else if rVal == v {
				return false
			}
		}
	}
	return true
}

// ParseTags takes a tag in the form foo=bar,baz!=bat and returns a TagFilter that will match the tags
func ParseTags(t string) (TagFilter, error) {
	tf := TagFilter{}
	tf.positiveTags = map[string]string{}
	tf.negativeTags = map[string]string{}
	var err error
	tags := strings.Split(t, ",")
	for _, tag := range tags {
		if match, _ := regexp.MatchString("^[a-zA-Z0-9_-]+!=[a-zA-Z0-9_-]+$", tag); match == true { // k!=v case
			kv := strings.Split(tag, "!=")
			tf.negativeTags[kv[0]] = kv[1]
		} else if match, _ := regexp.MatchString("^[a-zA-Z0-9_-]+=[a-zA-Z0-9_-]+$", tag); match == true { // k=v case
			kv := strings.Split(tag, "=")
			tf.positiveTags[kv[0]] = kv[1]
		} else { // malformed case
			err = fmt.Errorf("Malformed tag: %s. Expected tag in form k=v or k!=v", tag)
		}
	}
	return tf, err
}
