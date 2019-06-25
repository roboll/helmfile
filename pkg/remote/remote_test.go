package remote

import (
	"fmt"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/testhelper"
	"os"
	"testing"
)

func TestRemote_HttpsGitHub(t *testing.T) {
	cleanfs := map[string]string{
		"path/to/home": "",
	}
	cachefs := map[string]string{
		"path/to/home/.helmfile/cache/https_github_com_cloudposse_helmfiles_git.ref=0.40.0/releases/kiam.yaml": "foo: bar",
	}

	type testcase struct {
		files          map[string]string
		expectCacheHit bool
	}

	testcases := []testcase{
		{files: cleanfs, expectCacheHit: false},
		{files: cachefs, expectCacheHit: true},
	}

	for i := range testcases {
		testcase := testcases[i]

		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			testfs := testhelper.NewTestFs(testcase.files)

			hit := true

			get := func(wd, src, dst string) error {
				if wd != "path/to/home" {
					return fmt.Errorf("unexpected wd: %s", wd)
				}
				if src != "git::https://github.com/cloudposse/helmfiles.git?ref=0.40.0" {
					return fmt.Errorf("unexpected src: %s", src)
				}

				hit = false

				return nil
			}

			getter := &testGetter{
				get: get,
			}
			remote := &Remote{
				Logger:     helmexec.NewLogger(os.Stderr, "debug"),
				Home:       "path/to/home",
				Getter:     getter,
				ReadFile:   testfs.ReadFile,
				FileExists: testfs.FileExistsAt,
				DirExists:  testfs.DirectoryExistsAt,
			}

			// FYI, go-getter in the `dir` mode accepts URL like the below. So helmfile expects URLs similar to it:
			//   go-getter -mode dir git::https://github.com/cloudposse/helmfiles.git?ref=0.40.0 gettertest1/b

			// We use `@` to separate dir and the file path. This is a good idea borrowed from helm-git:
			//   https://github.com/aslafy-z/helm-git

			url := "git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref=0.40.0"
			file, err := remote.Locate(url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if file != "path/to/home/.helmfile/cache/https_github_com_cloudposse_helmfiles_git.ref=0.40.0/releases/kiam.yaml" {
				t.Errorf("unexpected file located: %s", file)
			}

			if testcase.expectCacheHit && !hit {
				t.Errorf("unexpected result: unexpected cache miss")
			}
			if !testcase.expectCacheHit && hit {
				t.Errorf("unexpected result: unexpected cache hit")
			}
		})
	}
}

func TestRemote_SShGitHub(t *testing.T) {
	cleanfs := map[string]string{
		"path/to/home": "",
	}
	cachefs := map[string]string{
		"path/to/home/.helmfile/cache/ssh_github_com_cloudposse_helmfiles_git.ref=0.40.0/releases/kiam.yaml": "foo: bar",
	}

	type testcase struct {
		files          map[string]string
		expectCacheHit bool
	}

	testcases := []testcase{
		{files: cleanfs, expectCacheHit: false},
		{files: cachefs, expectCacheHit: true},
	}

	for i := range testcases {
		testcase := testcases[i]

		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			testfs := testhelper.NewTestFs(testcase.files)

			hit := true

			get := func(wd, src, dst string) error {
				if wd != "path/to/home" {
					return fmt.Errorf("unexpected wd: %s", wd)
				}
				if src != "git::ssh://git@github.com/cloudposse/helmfiles.git?ref=0.40.0" {
					return fmt.Errorf("unexpected src: %s", src)
				}

				hit = false

				return nil
			}

			getter := &testGetter{
				get: get,
			}
			remote := &Remote{
				Logger:     helmexec.NewLogger(os.Stderr, "debug"),
				Home:       "path/to/home",
				Getter:     getter,
				ReadFile:   testfs.ReadFile,
				FileExists: testfs.FileExistsAt,
				DirExists:  testfs.DirectoryExistsAt,
			}

			url := "git::ssh://git@github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref=0.40.0"
			file, err := remote.Locate(url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if file != "path/to/home/.helmfile/cache/ssh_github_com_cloudposse_helmfiles_git.ref=0.40.0/releases/kiam.yaml" {
				t.Errorf("unexpected file located: %s", file)
			}

			if testcase.expectCacheHit && !hit {
				t.Errorf("unexpected result: unexpected cache miss")
			}
			if !testcase.expectCacheHit && hit {
				t.Errorf("unexpected result: unexpected cache hit")
			}
		})
	}
}

type testGetter struct {
	get func(wd, src, dst string) error
}

func (t *testGetter) Get(wd, src, dst string) error {
	return t.get(wd, src, dst)
}
