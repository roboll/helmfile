package testhelper

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/aryann/difflib"
)

func Diff(want, got string, context int) (string, bool) {
	records := difflib.Diff(
		strings.Split(want, "\n"),
		strings.Split(got, "\n"),
	)

	w := &bytes.Buffer{}

	changed := checkAndPrintRecords(w, records, context)

	return w.String(), changed
}

func checkAndPrintRecords(w io.Writer, records []difflib.DiffRecord, context int) bool {
	var changed bool
	if context >= 0 {
		distances := calculateDistances(records)
		omitting := false
		for i, diff := range records {
			if diff.Delta != difflib.Common {
				changed = true
			}
			if distances[i] > context {
				if !omitting {
					fmt.Fprintln(w, "...")
					omitting = true
				}
			} else {
				omitting = false
				fmt.Fprintln(w, formatRecord(diff))
			}
		}
	} else {
		for _, diff := range records {
			if diff.Delta != difflib.Common {
				changed = true
			}
			fmt.Fprintln(w, formatRecord(diff))
		}
	}
	return changed
}

func formatRecord(diff difflib.DiffRecord) string {
	var prefix string
	switch diff.Delta {
	case difflib.RightOnly:
		prefix = "+ "
	case difflib.LeftOnly:
		prefix = "- "
	case difflib.Common:
		prefix = "  "
	}

	return prefix + diff.Payload
}

// Shamelessly and thankfully copied from https://github.com/databus23/helm-diff/blob/99b8474af7726ca6f57b37b0b8b8f3cd36c991e8/diff/diff.go#L116
func calculateDistances(diffs []difflib.DiffRecord) map[int]int {
	distances := map[int]int{}

	// Iterate forwards through diffs, set 'distance' based on closest 'change' before this line
	change := -1
	for i, diff := range diffs {
		if diff.Delta != difflib.Common {
			change = i
		}
		distance := math.MaxInt32
		if change != -1 {
			distance = i - change
		}
		distances[i] = distance
	}

	// Iterate backwards through diffs, reduce 'distance' based on closest 'change' after this line
	change = -1
	for i := len(diffs) - 1; i >= 0; i-- {
		diff := diffs[i]
		if diff.Delta != difflib.Common {
			change = i
		}
		if change != -1 {
			distance := change - i
			if distance < distances[i] {
				distances[i] = distance
			}
		}
	}

	return distances
}
