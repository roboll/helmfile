// Package gotenv provides functionality to dynamically load the environment variables
package gotenv

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	// Pattern for detecting valid line format
	linePattern = `\A\s*(?:export\s+)?([\w\.]+)(?:\s*=\s*|:\s+?)('(?:\'|[^'])*'|"(?:\"|[^"])*"|[^#\n]+)?\s*(?:\s*\#.*)?\z`

	// Pattern for detecting valid variable within a value
	variablePattern = `(\\)?(\$)(\{?([A-Z0-9_]+)?\}?)`
)

// ErrFormat is an error for invalid line format
type ErrFormat struct {
	Message string
}

func (e ErrFormat) Error() string {
	return e.Message
}

// Env holds key/value pair of valid environment variable
type Env map[string]string

/*
Load is function to load a file or multiple files and then export the valid variables into environment variables if they are not exists.
When it's called with no argument, it will load `.env` file on the current path and set the environment variables.
Otherwise, it will loop over the filenames parameter and set the proper environment variables.
*/
func Load(filenames ...string) error {
	return loadenv(false, filenames...)
}

/*
MustLoad is similar function like Load but will panic when supplied files are not exist.
*/
func MustLoad(filenames ...string) {
	err := Load(filenames...)
	if err != nil {
		panic(err.Error())
	}
}

/*
OverLoad is function to load a file or multiple files and then export and override the valid variables into environment variables.
*/
func OverLoad(filenames ...string) error {
	return loadenv(true, filenames...)
}

/*
MustOverLoad is similar function like OverLoad but will panic when supplied files are not exist.
*/
func MustOverLoad(filenames ...string) {
	err := OverLoad(filenames...)
	if err != nil {
		panic(err.Error())
	}
}

/*
Apply is function to load an io Reader then export the valid variables into environment variables if they are not exist.
*/
func Apply(r io.Reader) error {
	return parset(r, false)
}

/*
OverApply is function to load an io Reader then export and override the valid variables into environment variables.
*/
func OverApply(r io.Reader) error {
	return parset(r, true)
}

func loadenv(override bool, filenames ...string) error {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}

	for _, filename := range filenames {
		f, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		err = parset(f, override)
		if err != nil {
			return err
		}
	}

	return nil
}

// parse and set :)
func parset(r io.Reader, override bool) error {
	env, err := StrictParse(r)
	if err != nil {
		return err
	}

	for key, val := range env {
		setenv(key, val, override)
	}

	return nil
}

func setenv(key, val string, override bool) {
	if override {
		os.Setenv(key, val)
	} else {
		if _, present := os.LookupEnv(key); !present {
			os.Setenv(key, val)
		}
	}
}

// Parse is a function to parse line by line any io.Reader supplied and returns the valid Env key/value pair of valid variables.
// It expands the value of a variable from environment variable, but does not set the value to the environment itself.
// This function is skipping any invalid lines and only processing the valid one.
func Parse(r io.Reader) Env {
	env, _ := StrictParse(r)
	return env
}

// StrictParse is a function to parse line by line any io.Reader supplied and returns the valid Env key/value pair of valid variables.
// It expands the value of a variable from environment variable, but does not set the value to the environment itself.
// This function is returning an error if there is any invalid lines.
func StrictParse(r io.Reader) (Env, error) {
	env := make(Env)
	scanner := bufio.NewScanner(r)

	i := 1
	bom := string([]byte{239, 187, 191})

	for scanner.Scan() {
		line := scanner.Text()

		if i == 1 {
			line = strings.TrimPrefix(line, bom)
		}

		i++

		err := parseLine(line, env)
		if err != nil {
			return env, err
		}
	}

	return env, nil
}

func parseLine(s string, env Env) error {
	rl := regexp.MustCompile(linePattern)
	rm := rl.FindStringSubmatch(s)

	if len(rm) == 0 {
		st := strings.TrimSpace(s)

		if (st == "") || strings.HasPrefix(st, "#") {
			return nil
		}

		if strings.HasPrefix(st, "export") {
			vs := strings.SplitN(st, " ", 2)

			if len(vs) > 1 {
				if _, ok := env[vs[1]]; !ok {
					return ErrFormat{Message: fmt.Sprintf("Line `%s` has an unset variable", st)}
				}
			}
		}

		return ErrFormat{Message: fmt.Sprintf("Line `%s` doesn't match format", s)}
	}

	key := rm[1]
	val := rm[2]

	// determine if string has quote prefix
	hdq := strings.HasPrefix(val, `"`)

	// determine if string has single quote prefix
	hsq := strings.HasPrefix(val, `'`)

	// trim whitespace
	val = strings.Trim(val, " ")

	// remove quotes '' or ""
	rq := regexp.MustCompile(`\A(['"])(.*)(['"])\z`)
	val = rq.ReplaceAllString(val, "$2")

	if hdq {
		val = strings.Replace(val, `\n`, "\n", -1)
		val = strings.Replace(val, `\r`, "\r", -1)

		// Unescape all characters except $ so variables can be escaped properly
		re := regexp.MustCompile(`\\([^$])`)
		val = re.ReplaceAllString(val, "$1")
	}

	rv := regexp.MustCompile(variablePattern)
	fv := func(s string) string {
		if strings.HasPrefix(s, "\\") {
			return strings.TrimPrefix(s, "\\")
		}

		if hsq {
			return s
		}

		sn := `(\$)(\{?([A-Z0-9_]+)\}?)`
		rn := regexp.MustCompile(sn)
		mn := rn.FindStringSubmatch(s)

		if len(mn) == 0 {
			return s
		}

		v := mn[3]

		replace, ok := env[v]
		if !ok {
			replace = os.Getenv(v)
		}

		return replace
	}

	val = rv.ReplaceAllStringFunc(val, fv)

	if strings.Contains(val, "=") {
		if !(val == "\n" || val == "\r") {
			kv := strings.Split(val, "\n")

			if len(kv) == 1 {
				kv = strings.Split(val, "\r")
			}

			if len(kv) > 1 {
				val = kv[0]

				for i := 1; i < len(kv); i++ {
					parseLine(kv[i], env)
				}
			}
		}
	}

	env[key] = val
	return nil
}
