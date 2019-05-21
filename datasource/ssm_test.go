package datasource

import (
	"flag"
	"io/ioutil"
	"os"
	"testing"

	"github.com/urfave/cli"
)

const (
	ssmTestRegion = "SSM_TEST_REGION"
)

func ssmTestPrepare() (tmpfileName string, err error) {
	os.Setenv("AWS_REGION", ssmTestRegion)

	tmpfile, err := ioutil.TempFile("", "ssm_test.yml")
	if err != nil {
		return
	}
	defer tmpfile.Close()
	tmpfileName = tmpfile.Name()

	content := `---
ssm:
  - name: east1
    prefix: /dev/us-east-1/redis
    region: us-east-1
  - name: west2
    prefix: /dev/us-west-2/redis
    region: us-west-2

releases:
  - name: redis-test
    chart: stable/redis
    values:
    - fake_key:
        redis_pass_east: {{ ssm "east1:redis_password" }}
        redis_pass_west: {{ ssm "west2:redis_password" }}
        redis_pass_seast: {{ ssm "/dev/ap-southeast-1/redis/redis_password" }}
`
	if _, err = tmpfile.Write([]byte(content)); err != nil {
		return
	}

	flagSet := flag.NewFlagSet("test", 0)
	flagSet.String("file", tmpfile.Name(), "A Helmfile for testing")
	c := cli.NewContext(nil, flagSet, nil)
	PrepareAll(c)

	if err = ssmGetSpecs(); err != nil {
		return
	}

	return
}

func Test_ssmAssembleKey(t *testing.T) {
	tmpfileName, err := ssmTestPrepare()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfileName)

	type args struct {
		key string
	}
	tests := []struct {
		name             string
		args             args
		wantAssembledKey string
		wantErr          bool
	}{
		{
			args:             args{key: "testing_123"},
			wantAssembledKey: "testing_123",
		},
		{
			args:             args{key: "/path/to/testing_123"},
			wantAssembledKey: "/path/to/testing_123",
		},
		{
			args:             args{key: "east1:testing_123"},
			wantAssembledKey: "/dev/us-east-1/redis/testing_123",
		},
		{
			args:             args{key: "west2:testing_123"},
			wantAssembledKey: "/dev/us-west-2/redis/testing_123",
		},
		{
			args:    args{key: "doesnotexist:testing_123"},
			wantErr: true,
		},
		{
			args:             args{key: ""},
			wantAssembledKey: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAssembledKey, err := ssmAssembleKey(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ssmAssembleKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAssembledKey != tt.wantAssembledKey {
				t.Errorf("ssmAssembleKey() = %v, want %v", gotAssembledKey, tt.wantAssembledKey)
			}
		})
	}
}

func Test_ssmGetRegionFromKey(t *testing.T) {
	tmpfileName, err := ssmTestPrepare()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfileName)

	type args struct {
		key string
	}
	tests := []struct {
		name       string
		args       args
		wantRegion string
	}{
		{
			args:       args{key: "east1:testing_123"},
			wantRegion: "us-east-1",
		},
		{
			args:       args{key: "west2:testing_123"},
			wantRegion: "us-west-2",
		},
		{
			args:       args{key: "/path/to/testing_123"},
			wantRegion: "",
		},
		{
			args:       args{key: "doesnotexist:testing_123"},
			wantRegion: "",
		},
		{
			args:       args{key: ""},
			wantRegion: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotRegion := ssmGetRegionFromKey(tt.args.key); gotRegion != tt.wantRegion {
				t.Errorf("ssmGetRegionFromKey() = %v, want %v", gotRegion, tt.wantRegion)
			}
		})
	}
}
