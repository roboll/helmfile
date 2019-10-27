go-prettytable
==============

go-prettytable is a library for Golang to build a simple text table with a
multibyte, doublewidth character support.

[![GoDoc](https://godoc.org/github.com/tatsushid/go-prettytable?status.svg)](https://godoc.org/github.com/tatsushid/go-prettytable)

## Installation

Install and update this go package with `go get -u
github.com/tatsushid/go-prettytable`

## Examples

Import this package and use

```go
tbl, err := prettytable.NewTable([]prettytable.Column{
	{Header: "COL1"},
	{Header: "COL2", MinWidth: 6},
	{Header: "COL3", AlignRight: true},
}...)
if err != nil {
	panic(err)
}
tbl.Separator = " | "
tbl.AddRow("foo", "bar", "baz")
tbl.AddRow(1, 2.3, 4)
tbl.Print()
```

It outputs

```
COL1 | COL2   | COL3
foo  | bar    |  baz
1    | 2.3    |    4
```

Also it can be used with multibyte, doublewidth characters

```go
tbl, err := prettytable.NewTable([]prettytable.Column{
	{Header: "名前"},
	{Header: "個数", AlignRight: true},
}...)
if err != nil {
	panic(err)
}
tbl.Separator = " | "
tbl.AddRow("りんご", 5)
tbl.AddRow("みかん", 3)
tbl.AddRow("柿", 2)
tbl.Print()
```

It outputs (may not be displayed correctly with proportional fonts but it
is displeyed good on terminal)

```
名前   | 個数
りんご |    5
みかん |    3
柿     |    2
```

For more detail, please see [godoc][godoc].

## See Also
- [gotabulate](https://github.com/bndr/gotabulate)
- [go-texttable](https://github.com/syohex/go-texttable)

## License
go-prettytable is under MIT License. See the [LICENSE][license] file for
details.

[godoc]: http://godoc.org/github.com/tatsushid/go-prettytable
[license]: https://github.com/tatsushid/go-prettytable/blob/master/LICENSE
