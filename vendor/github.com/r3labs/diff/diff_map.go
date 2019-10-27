/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package diff

import (
	"fmt"
	"reflect"
)

func (d *Differ) diffMap(path []string, a, b reflect.Value) error {
	if a.Kind() == reflect.Invalid {
		return d.mapValues(CREATE, path, b)
	}

	if b.Kind() == reflect.Invalid {
		return d.mapValues(DELETE, path, a)
	}

	c := NewComparativeList()

	for _, k := range a.MapKeys() {
		ae := a.MapIndex(k)
		c.addA(k.Interface(), &ae)
	}

	for _, k := range b.MapKeys() {
		be := b.MapIndex(k)
		c.addB(k.Interface(), &be)
	}

	return d.diffComparative(path, c)
}

func (d *Differ) mapValues(t string, path []string, a reflect.Value) error {
	if t != CREATE && t != DELETE {
		return ErrInvalidChangeType
	}

	if a.Kind() == reflect.Ptr {
		a = reflect.Indirect(a)
	}

	if a.Kind() != reflect.Map {
		return ErrTypeMismatch
	}

	x := reflect.New(a.Type()).Elem()

	for _, k := range a.MapKeys() {
		ae := a.MapIndex(k)
		xe := x.MapIndex(k)

		err := d.diff(append(path, fmt.Sprint(k.Interface())), xe, ae)
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(d.cl); i++ {
		// only swap changes on the relevant map
		if pathmatch(path, d.cl[i].Path) {
			d.cl[i] = swapChange(t, d.cl[i])
		}
	}

	return nil
}
