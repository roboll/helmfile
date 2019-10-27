# diff

A library for diffing golang structures and values.

Utilizing field tags and reflection, it is able to compare two structures of the same type and create a changelog of all modified values. The produced changelog can easily be serialized to json.

## Build status

* Master [![CircleCI](https://circleci.com/gh/r3labs/diff/tree/master.svg?style=svg)](https://circleci.com/gh/r3labs/diff/tree/master)

## Installation

```
go get github.com/r3labs/diff
```

## Changelog Format

When diffing two structures using `Diff`, a changelog will be produced. Any detected changes will populate the changelog array with a Change type:

```go
type Change struct {
	Type string      // The type of change detected; can be one of create, update or delete
	Path []string    // The path of the detected change; will contain any field name or array index that was part of the traversal
	From interface{} // The original value that was present in the "from" structure
	To   interface{} // The new value that was detected as a change in the "to" structure
}
```

Given the example below, we are diffing two slices where the third element has been removed:

```go
from := []int{1, 2, 3, 4}
to := []int{1, 2, 4}

changelog, _ := diff.Diff(from, to)
```

The resultant changelog should contain one change:

```go
Change{
    Type: "delete",
    Path: ["2"],
    From: 3,
    To:   nil,
}
```

## Supported Types

A diffable value can be/contain any of the following types:

* struct
* slice
* string
* int
* bool
* map
* pointer

### Tags

In order for struct fields to be compared, they must be tagged with a given name. All tag values are prefixed with `diff`. i.e. `diff:"items"`.

* `-` : In the event that you want to exclude a value from the diff, you can use the tag `diff:"-"` and the field will be ignored.

* `identifier` : If you need to compare arrays by a matching identifier and not based on order, you can specify the `identifier` tag. If an identifiable element is found in both the from and to structures, they will be directly compared. i.e. `diff:"name,identifier"`

* `immutable` : Will omit this struct field from diffing. When using `diff.StructValues()` these values will be added to the returned changelog. It's usecase is for when we have nothing to compare a struct to and want to show all of its relevant values.

## Usage

### Basic Example

Diffing a basic set of values can be accomplished using the diff functions. Any items that specify a "diff" tag using a name will be compared.

```go
import "github.com/r3labs/diff"

type Order struct {
    ID    string `diff:"id"`
    Items []int  `diff:"items"`
}

func main() {
    a := Order{
        ID: "1234",
        Items: []int{1, 2, 3, 4},
    }

    b := Order{
        ID: "1234",
        Items: []int{1, 2, 4},
    }

    changelog, err := diff.Diff(a, b)
    ...
}
```

In this example, the output generated in the changelog will indicate that the third element with a value of '3' was removed from items.
When marshalling the changelog to json, the output will look like:

```json
[
    {
        "type": "delete",
        "path": ["items", "2"],
        "from": 3,
        "to": null
    }
]
```

### Options and Configuration

You can create a new instance of a differ that allows options to be set.

```go
import "github.com/r3labs/diff"

type Order struct {
    ID    string `diff:"id"`
    Items []int  `diff:"items"`
}

func main() {
    a := Order{
        ID: "1234",
        Items: []int{1, 2, 3, 4},
    }

    b := Order{
        ID: "1234",
        Items: []int{1, 2, 4},
    }

	d, err := diff.NewDiffer(diff.SliceOrdering(true))
	if err != nil {
		panic(err)
	}

    changelog, err := d.Diff(a, b)
    ...
}
```

Supported options are:

`SliceOrdering` ensures that the ordering of items in a slice is taken into account


## Running Tests

```
make test
```

## Contributing

Please read through our
[contributing guidelines](CONTRIBUTING.md).
Included are directions for opening issues, coding standards, and notes on
development.

Moreover, if your pull request contains patches or features, you must include
relevant unit tests.

## Versioning

For transparency into our release cycle and in striving to maintain backward
compatibility, this project is maintained under [the Semantic Versioning guidelines](http://semver.org/).

## Copyright and License

Code and documentation copyright since 2015 r3labs.io authors.

Code released under
[the Mozilla Public License Version 2.0](LICENSE).
