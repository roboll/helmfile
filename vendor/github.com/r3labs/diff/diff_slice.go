/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package diff

import (
	"reflect"
)

func (d *Differ) diffSlice(path []string, a, b reflect.Value) error {
	if a.Kind() != b.Kind() {
		return ErrTypeMismatch
	}

	if comparative(a, b) {
		return d.diffSliceComparative(path, a, b)
	}

	return d.diffSliceGeneric(path, a, b)
}

func (d *Differ) diffSliceGeneric(path []string, a, b reflect.Value) error {
	missing := NewComparativeList()

	slice := sliceTracker{}
	for i := 0; i < a.Len(); i++ {
		ae := a.Index(i)

		if (d.SliceOrdering && !hasAtSameIndex(b, ae, i)) || (!d.SliceOrdering && !slice.has(b, ae)) {
			missing.addA(i, &ae)
		}
	}

	slice = sliceTracker{}
	for i := 0; i < b.Len(); i++ {
		be := b.Index(i)

		if (d.SliceOrdering && !hasAtSameIndex(a, be, i)) || (!d.SliceOrdering && !slice.has(a, be)) {
			missing.addB(i, &be)
		}
	}

	// fallback to comparing based on order in slice if item is missing
	if len(missing.keys) == 0 {
		return nil
	}

	return d.diffComparative(path, missing)
}

func (d *Differ) diffSliceComparative(path []string, a, b reflect.Value) error {
	c := NewComparativeList()

	for i := 0; i < a.Len(); i++ {
		ae := a.Index(i)
		ak := getFinalValue(ae)

		id := identifier(ak)
		if id != nil {
			c.addA(id, &ae)
		}
	}

	for i := 0; i < b.Len(); i++ {
		be := b.Index(i)
		bk := getFinalValue(be)

		id := identifier(bk)
		if id != nil {
			c.addB(id, &be)
		}
	}

	return d.diffComparative(path, c)
}

// keeps track of elements that have already been matched, to stop duplicate matches from occuring
type sliceTracker []bool

func (st *sliceTracker) has(s, v reflect.Value) bool {
	if len(*st) != s.Len() {
		(*st) = make([]bool, s.Len(), s.Len())
	}

	for i := 0; i < s.Len(); i++ {
		// skip already matched elements
		if (*st)[i] {
			continue
		}

		x := s.Index(i)
		if reflect.DeepEqual(x.Interface(), v.Interface()) {
			(*st)[i] = true
			return true
		}
	}

	return false
}

func getFinalValue(t reflect.Value) reflect.Value {
	switch t.Kind() {
	case reflect.Interface:
		return getFinalValue(t.Elem())
	case reflect.Ptr:
		return getFinalValue(reflect.Indirect(t))
	default:
		return t
	}
}

func hasAtSameIndex(s, v reflect.Value, atIndex int) bool {
	// check the element in the slice at atIndex to see if it matches value, if it is a valid index into the slice
	if atIndex < s.Len() {
		x := s.Index(atIndex)
		return reflect.DeepEqual(x.Interface(), v.Interface())
	}

	return false
}
