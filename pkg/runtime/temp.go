package runtime

import (
	"fmt"
	"hash/fnv"
	"os"

	"github.com/davecgh/go-spew/spew"
)

var (
	uniqueTempDirs map[string]string
)

func init() {
	uniqueTempDirs = make(map[string]string)
}

func UniqueTempDir(pattern string) string {
	// Set a default pattern
	if pattern == "" {
		pattern = "helmfile"
	}
	if _, ok := uniqueTempDirs[pattern]; !ok {
		uniqueTempDirs[pattern] = TempDir(pattern)
	}

	return uniqueTempDirs[pattern]
}

func TempDir(pattern string) string {
	// Set a default pattern
	if pattern == "" {
		pattern = "helmfile"
	}
	var err error
	var workDir string
	workDir, err = os.MkdirTemp(os.Getenv("HELMFILE_TEMPDIR"), pattern)
	if err != nil {
		return os.TempDir()
	}
	return workDir
}

func TempFileUniqueTempDir(dirPattern, filePattern string) (*os.File, error) {
	return os.CreateTemp(UniqueTempDir(dirPattern), filePattern)
}

func TempFile(dirPattern, filePattern string) (*os.File, error) {
	return os.CreateTemp(TempDir(dirPattern), filePattern)
}

func HashObject(obj interface{}) (string, error) {
	hash := fnv.New32a()

	hash.Reset()

	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hash, "%#v", obj)

	sum := fmt.Sprint(hash.Sum32())

	return SafeEncodeString(sum), nil
}
