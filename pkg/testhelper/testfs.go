package testhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TestFs struct {
	Cwd   string
	dirs  map[string]bool
	files map[string]string

	GlobFixtures map[string][]string

	fileReaderCalls int
	successfulReads []string
}

func NewTestFs(files map[string]string) *TestFs {
	dirs := map[string]bool{}
	for abs, _ := range files {
		for d := filepath.Dir(abs); !dirs[d]; d = filepath.Dir(d) {
			dirs[d] = true
			fmt.Fprintf(os.Stderr, "testfs: recognized dir: %s\n", d)
		}
	}
	return &TestFs{
		Cwd:   "/path/to",
		dirs:  dirs,
		files: files,

		successfulReads: []string{},

		GlobFixtures: map[string][]string{},
	}
}

func (f *TestFs) FileExistsAt(path string) bool {
	var ok bool
	if strings.Contains(path, "/") {
		_, ok = f.files[path]
	} else {
		_, ok = f.files[filepath.Join(f.Cwd, path)]
	}
	return ok
}

func (f *TestFs) FileExists(path string) (bool, error) {
	return f.FileExistsAt(path), nil
}

func (f *TestFs) DirectoryExistsAt(path string) bool {
	var ok bool
	if strings.Contains(path, "/") {
		_, ok = f.dirs[path]
	} else {
		_, ok = f.dirs[filepath.Join(f.Cwd, path)]
	}
	return ok
}

func (f *TestFs) ReadFile(filename string) ([]byte, error) {
	var str string
	var ok bool
	if filename[0] == '/' {
		str, ok = f.files[filename]
	} else {
		str, ok = f.files[filepath.Join(f.Cwd, filename)]
	}
	if !ok {
		return []byte(nil), os.ErrNotExist
	}

	f.fileReaderCalls += 1

	f.successfulReads = append(f.successfulReads, filename)

	return []byte(str), nil
}

func (f *TestFs) SuccessfulReads() []string {
	return f.successfulReads
}

func (f *TestFs) FileReaderCalls() int {
	return f.fileReaderCalls
}

func (f *TestFs) Glob(relPattern string) ([]string, error) {
	var pattern string
	if relPattern[0] == '/' {
		pattern = relPattern
	} else {
		pattern = filepath.Join(f.Cwd, relPattern)
	}

	fixtures, ok := f.GlobFixtures[pattern]
	if ok {
		return fixtures, nil
	}

	matches := []string{}
	for name, _ := range f.files {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, name)
		}
	}
	return matches, nil
}

func (f *TestFs) Abs(path string) (string, error) {
	var p string
	if path[0] == '/' {
		p = path
	} else {
		p = filepath.Join(f.Cwd, path)
	}
	return filepath.Clean(p), nil
}

func (f *TestFs) Getwd() (string, error) {
	return f.Cwd, nil
}

func (f *TestFs) Chdir(dir string) error {
	if _, ok := f.dirs[dir]; ok {
		f.Cwd = dir
		return nil
	}
	return fmt.Errorf("unexpected chdir \"%s\"", dir)
}
