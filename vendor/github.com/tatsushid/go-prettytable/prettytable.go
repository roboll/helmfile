// Package prettytable is a library for Golang to build a simple text table
// with a multibyte, doublewidth character support.
package prettytable

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// Package wide column separator, used as default when NewTable is called.
var Separator = " "

// Column is used for column definitions of a table
type Column struct {
	// Column name used as its header
	Header string
	// If this value is true, the column is aligned to the right.
	AlignRight bool
	// Minimal width of column. If Header or column value's length is
	// larger than it, the column width is extended to the length.
	MinWidth int
	// Maximal width of column. If Header or column value's length is
	// larger than it, the header or value is truncated
	MaxWidth int
	width    int
}

// Table represents a table itself and has its option values
type Table struct {
	// If this value is true, a header line isn't generated
	NoHeader bool
	// Column separator characters. Separator package value is default
	Separator string
	columns   []Column
	rows      [][]string
}

func truncateString(str string, width int) string {
	w := 0
	b := []byte(str)
	var buf bytes.Buffer
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		rw := runewidth.RuneWidth(r)
		if w+rw > width {
			break
		}
		buf.WriteRune(r)
		w += rw
		b = b[size:]
	}
	return buf.String()
}

// NewTable defines a table with columns and returns *Table. It returns an
// error if no columns are passed or passed invalid columns, for example,
// MinWidth is larger than MaxWidth
func NewTable(cols ...Column) (*Table, error) {
	if len(cols) == 0 {
		return nil, errors.New("no columns")
	}
	t := new(Table)
	t.Separator = Separator
	t.columns = make([]Column, len(cols))
	copy(t.columns, cols)
	for i, c := range cols {
		if c.MinWidth > 0 && c.MaxWidth > 0 && c.MinWidth > c.MaxWidth {
			return nil, errors.New("invalid Column. MaxWidth must be larger than MinWidth")
		}
		t.columns[i].width = runewidth.StringWidth(c.Header)
		if c.MinWidth > 0 && c.MinWidth > t.columns[i].width {
			t.columns[i].width = c.MinWidth
		}
		if c.MaxWidth > 0 && c.MaxWidth < t.columns[i].width {
			t.columns[i].Header = truncateString(c.Header, c.MaxWidth)
			t.columns[i].width = c.MaxWidth
		}
	}
	return t, nil
}

func convertToString(v interface{}) (string, error) {
	switch vv := v.(type) {
	case fmt.Stringer:
		return vv.String(), nil
	case int:
		return strconv.FormatInt(int64(vv), 10), nil
	case int8:
		return strconv.FormatInt(int64(vv), 10), nil
	case int16:
		return strconv.FormatInt(int64(vv), 10), nil
	case int32:
		return strconv.FormatInt(int64(vv), 10), nil
	case int64:
		return strconv.FormatInt(vv, 10), nil
	case uint:
		return strconv.FormatUint(uint64(vv), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(vv), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(vv), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(vv), 10), nil
	case uint64:
		return strconv.FormatUint(vv, 10), nil
	case float32:
		return strconv.FormatFloat(float64(vv), 'g', -1, 32), nil
	case float64:
		return strconv.FormatFloat(vv, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(vv), nil
	case string:
		return vv, nil
	case []byte:
		return string(vv), nil
	case []rune:
		return string(vv), nil
	default:
		return "", errors.New("can't convert the value")
	}
}

/*
AddRow adds a row with given values. It returns an error if no values are
passed or a number of values is larger than a number of columns.

It converts values into strings by following rules

	- If a value fulfills fmt.Stringer interface, it is converted into its
	  String() function result
	- If a value is an integer or a float, it is converted into a decimal
	  number string.
	- If a value is a bool, it is converted into "true" or "false" string
	- If a value is a string, it is used as is.
	- If a value is a []byte or []rune, it is converted int string
	- Otherwise, an error is returned
*/
func (t *Table) AddRow(vars ...interface{}) error {
	if len(vars) == 0 {
		return errors.New("no row data")
	} else if len(vars) > len(t.columns) {
		return errors.New("a number of row data must be less than a number of columns")
	}
	var row []string
	for i, v := range vars {
		s, err := convertToString(v)
		if err != nil {
			return err
		}
		row = append(row, s)
		strlen := runewidth.StringWidth(s)
		if strlen > t.columns[i].width {
			if t.columns[i].MaxWidth > 0 && t.columns[i].MaxWidth < strlen {
				row[i] = truncateString(s, t.columns[i].MaxWidth)
				t.columns[i].width = t.columns[i].MaxWidth
			} else {
				t.columns[i].width = strlen
			}
		}
	}
	t.rows = append(t.rows, row)
	return nil
}

// Bytes returns a generated text table []byte
func (t *Table) Bytes() []byte {
	var buf bytes.Buffer
	addCell := func(i int, s string, max int) []byte {
		var cell bytes.Buffer
		if i > 0 {
			cell.WriteString(t.Separator)
		}
		w := runewidth.StringWidth(s)
		sp := strings.Repeat(" ", t.columns[i].width-w)
		if t.columns[i].AlignRight {
			cell.WriteString(sp + s)
		} else {
			cell.WriteString(s)
			if i < max {
				cell.WriteString(sp)
			}
		}
		return cell.Bytes()
	}
	if !t.NoHeader {
		last := len(t.columns) - 1
		for i, c := range t.columns {
			buf.Write(addCell(i, c.Header, last))
		}
		buf.WriteByte('\n')
	}
	for _, row := range t.rows {
		last := len(row) - 1
		for i, s := range row {
			buf.Write(addCell(i, s, last))
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// String returns a generated text table string
func (t *Table) String() string {
	return string(t.Bytes())
}

// WriteTo writes a generated text table to a writer. It returns the number of
// bytes written. Any errors encountered during the write is also returned.
func (t *Table) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(t.Bytes())
	return int64(n), err
}

// Print prints a generated text table to os.Stdout. It returns the number of
// bytes written. Any errors encountered during the write is also returned.
func (t *Table) Print() (n int, err error) {
	return os.Stdout.Write(t.Bytes())
}
