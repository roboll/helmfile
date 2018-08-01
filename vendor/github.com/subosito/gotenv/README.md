# gotenv

[![Build Status](https://travis-ci.org/subosito/gotenv.svg?branch=master)](https://travis-ci.org/subosito/gotenv)
[![Coverage Status](https://img.shields.io/codecov/c/github/subosito/gotenv/master.svg?maxAge=2592000)](https://codecov.io/gh/subosito/gotenv)
[![Go Report Card](https://goreportcard.com/badge/subosito/gotenv)](https://goreportcard.com/report/subosito/gotenv)
[![GoDoc](https://godoc.org/github.com/subosito/gotenv?status.svg)](https://godoc.org/github.com/subosito/gotenv)

Load environment variables dynamically in Go.

## Installation

```bash
$ go get github.com/subosito/gotenv
```

## Usage

Store your configuration to `.env` file on your root directory of your project:

```
APP_ID=1234567
APP_SECRET=abcdef
```

You may also add `export` in front of each line so you can `source` the file in bash:

```bash
export APP_ID=1234567
export APP_SECRET=abcdef
```

Put the gotenv package on your `import` statement:

```go
import "github.com/subosito/gotenv"
```

Then somewhere on your application code, put:

```go
gotenv.Load()
```

Behind the scene it will then load `.env` file and export the valid variables to the environment variables. Make sure you call the method as soon as possible to ensure all variables are loaded, say, put it on `init()` function.

Once loaded you can use `os.Getenv()` to get the value of the variable.

Here's the final example:

```go
package main

import (
	"github.com/subosito/gotenv"
	"log"
	"os"
)

func init() {
	gotenv.Load()
}

func main() {
	log.Println(os.Getenv("APP_ID"))     // "1234567"
	log.Println(os.Getenv("APP_SECRET")) // "abcdef"
}
```

You can also load other than `.env` file if you wish. Just supply filenames when calling `Load()`:

```go
gotenv.Load(".env.production", "credentials")
```

That's it :)

### Another Scenario

Just in case you want to parse environment variables from any `io.Reader`, gotenv keeps its `Parse()` function as public API so you can utilize that.

```go
// import "strings"

pairs := gotenv.Parse(strings.NewReader("FOO=test\nBAR=$FOO"))
// gotenv.Env{"FOO": "test", "BAR": "test"}

pairs = gotenv.Parse(strings.NewReader(`FOO="bar"`))
// gotenv.Env{"FOO": "bar"}
```

Parse ignores invalid lines and returns `Env` of valid environment variables.

## Notes

The gotenv package is a Go port of [`dotenv`](https://github.com/bkeepers/dotenv) project. Most logic and regexp pattern is taken from there and aims will be compatible as close as possible.
