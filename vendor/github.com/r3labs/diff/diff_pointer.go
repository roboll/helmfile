/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package diff

import (
	"reflect"
)

func (d *Differ) diffPtr(path []string, a, b reflect.Value) error {
	if a.Kind() != b.Kind() {
		if a.Kind() == reflect.Invalid {
			if !b.IsNil() {
				return d.diff(path, reflect.ValueOf(nil), reflect.Indirect(b))
			}
		}

		if b.Kind() == reflect.Invalid {
			if !a.IsNil() {
				return d.diff(path, reflect.Indirect(a), reflect.ValueOf(nil))
			}
		}

		return ErrTypeMismatch
	}

	if a.IsNil() && b.IsNil() {
		return nil
	}

	if a.IsNil() {
		d.cl.add(UPDATE, path, nil, b.Interface())
		return nil
	}

	if b.IsNil() {
		d.cl.add(UPDATE, path, a.Interface(), nil)
		return nil
	}

	return d.diff(path, reflect.Indirect(a), reflect.Indirect(b))
}
