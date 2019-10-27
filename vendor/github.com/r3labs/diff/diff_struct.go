/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package diff

import (
	"reflect"
	"time"
)

func (d *Differ) diffStruct(path []string, a, b reflect.Value) error {
	if areType(a, b, reflect.TypeOf(time.Time{})) {
		return d.diffTime(path, a, b)
	}

	if a.Kind() == reflect.Invalid {
		if d.DisableStructValues {
			d.cl.add(CREATE, path, nil, b.Interface())
			return nil
		}
		return d.structValues(CREATE, path, b)
	}

	if b.Kind() == reflect.Invalid {
		if d.DisableStructValues {
			d.cl.add(DELETE, path, a.Interface(), nil)
			return nil
		}
		return d.structValues(DELETE, path, a)
	}

	for i := 0; i < a.NumField(); i++ {
		field := a.Type().Field(i)
		tname := tagName(field)

		if tname == "-" || hasTagOption(field, "immutable") {
			continue
		}

		if tname == "" {
			tname = field.Name
		}

		af := a.Field(i)
		bf := b.FieldByName(field.Name)

		fpath := copyAppend(path, tname)

		err := d.diff(fpath, af, bf)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Differ) structValues(t string, path []string, a reflect.Value) error {
	var nd Differ

	if t != CREATE && t != DELETE {
		return ErrInvalidChangeType
	}

	if a.Kind() == reflect.Ptr {
		a = reflect.Indirect(a)
	}

	if a.Kind() != reflect.Struct {
		return ErrTypeMismatch
	}

	x := reflect.New(a.Type()).Elem()

	for i := 0; i < a.NumField(); i++ {

		field := a.Type().Field(i)
		tname := tagName(field)

		if tname == "-" {
			continue
		}

		if tname == "" {
			tname = field.Name
		}

		af := a.Field(i)
		xf := x.FieldByName(field.Name)

		fpath := copyAppend(path, tname)

		err := nd.diff(fpath, xf, af)
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(nd.cl); i++ {
		(d.cl) = append(d.cl, swapChange(t, nd.cl[i]))
	}

	return nil
}
