package app

import (
	"bufio"
	"bytes"
	"github.com/roboll/helmfile/pkg/exectest"
	"github.com/roboll/helmfile/pkg/helmexec"
	"github.com/roboll/helmfile/pkg/testhelper"
	"github.com/variantdev/vals"
	"go.uber.org/zap"
	"io"
	"path/filepath"
	"sync"
	"testing"
)

type destroyConfig struct {
	args        string
	concurrency int
	interactive bool
	logger      *zap.SugaredLogger
}

func (d destroyConfig) Args() string {
	return d.args
}

func (d destroyConfig) Interactive() bool {
	return d.interactive
}

func (d destroyConfig) Logger() *zap.SugaredLogger {
	return d.logger
}

func (d destroyConfig) Concurrency() int {
	return d.concurrency
}

func TestDestroy(t *testing.T) {
	testcases := []struct {
		name        string
		loc         string
		ns          string
		concurrency int
		error       string
		files       map[string]string
		selectors   []string
		lists       map[exectest.ListKey]string
		diffs       map[exectest.DiffKey]error
		upgraded    []exectest.Release
		deleted     []exectest.Release
		log         string
	}{
		//
		// complex test cases for smoke testing
		//
		{
			name: "smoke",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: database
  chart: charts/mysql
  needs:
  - logging
- name: frontend-v1
  chart: charts/frontend
  installed: false
  needs:
  - servicemesh
  - logging
  - backend-v1
- name: frontend-v2
  chart: charts/frontend
  needs:
  - servicemesh
  - logging
  - backend-v2
- name: frontend-v3
  chart: charts/frontend
  needs:
  - servicemesh
  - logging
  - backend-v2
- name: backend-v1
  chart: charts/backend
  installed: false
  needs:
  - servicemesh
  - logging
  - database
  - anotherbackend
- name: backend-v2
  chart: charts/backend
  needs:
  - servicemesh
  - logging
  - database
  - anotherbackend
- name: anotherbackend
  chart: charts/anotherbackend
  needs:
  - servicemesh
  - logging
  - database
- name: servicemesh
  chart: charts/istio
  needs:
  - logging
- name: logging
  chart: charts/fluent-bit
- name: front-proxy
  chart: stable/envoy
`,
			},
			diffs: map[exectest.DiffKey]error{},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^frontend-v1$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
`,
				exectest.ListKey{Filter: "^frontend-v2$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
frontend-v2 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	frontend-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^frontend-v3$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
frontend-v3 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	frontend-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^backend-v1$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
`,
				exectest.ListKey{Filter: "^backend-v2$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
backend-v2 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	backend-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^logging$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
logging	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	fluent-bit-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^front-proxy$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
front-proxy 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	envoy-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^servicemesh$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
servicemesh 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	istio-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^database$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
database 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mysql-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^anotherbackend$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
anotherbackend 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	anotherbackend-3.1.0	3.1.0      	default
`,
			},
			// Disable concurrency to avoid in-deterministic result
			concurrency: 1,
			upgraded:    []exectest.Release{},
			deleted: []exectest.Release{
				{Name: "frontend-v3", Flags: []string{}},
				{Name: "frontend-v2", Flags: []string{}},
				{Name: "frontend-v1", Flags: []string{}},
				{Name: "backend-v2", Flags: []string{}},
				{Name: "backend-v1", Flags: []string{}},
				{Name: "anotherbackend", Flags: []string{}},
				{Name: "database", Flags: []string{}},
				{Name: "servicemesh", Flags: []string{}},
				{Name: "front-proxy", Flags: []string{}},
				{Name: "logging", Flags: []string{}},
			},
			log: `processing file "helmfile.yaml" in directory "."
first-pass rendering starting for "helmfile.yaml.part.0": inherited=&{default map[] map[]}, overrode=<nil>
first-pass uses: &{default map[] map[]}
first-pass rendering output of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: database
 3:   chart: charts/mysql
 4:   needs:
 5:   - logging
 6: - name: frontend-v1
 7:   chart: charts/frontend
 8:   installed: false
 9:   needs:
10:   - servicemesh
11:   - logging
12:   - backend-v1
13: - name: frontend-v2
14:   chart: charts/frontend
15:   needs:
16:   - servicemesh
17:   - logging
18:   - backend-v2
19: - name: frontend-v3
20:   chart: charts/frontend
21:   needs:
22:   - servicemesh
23:   - logging
24:   - backend-v2
25: - name: backend-v1
26:   chart: charts/backend
27:   installed: false
28:   needs:
29:   - servicemesh
30:   - logging
31:   - database
32:   - anotherbackend
33: - name: backend-v2
34:   chart: charts/backend
35:   needs:
36:   - servicemesh
37:   - logging
38:   - database
39:   - anotherbackend
40: - name: anotherbackend
41:   chart: charts/anotherbackend
42:   needs:
43:   - servicemesh
44:   - logging
45:   - database
46: - name: servicemesh
47:   chart: charts/istio
48:   needs:
49:   - logging
50: - name: logging
51:   chart: charts/fluent-bit
52: - name: front-proxy
53:   chart: stable/envoy
54: 

first-pass produced: &{default map[] map[]}
first-pass rendering result of "helmfile.yaml.part.0": {default map[] map[]}
vals:
map[]
defaultVals:[]
second-pass rendering result of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: database
 3:   chart: charts/mysql
 4:   needs:
 5:   - logging
 6: - name: frontend-v1
 7:   chart: charts/frontend
 8:   installed: false
 9:   needs:
10:   - servicemesh
11:   - logging
12:   - backend-v1
13: - name: frontend-v2
14:   chart: charts/frontend
15:   needs:
16:   - servicemesh
17:   - logging
18:   - backend-v2
19: - name: frontend-v3
20:   chart: charts/frontend
21:   needs:
22:   - servicemesh
23:   - logging
24:   - backend-v2
25: - name: backend-v1
26:   chart: charts/backend
27:   installed: false
28:   needs:
29:   - servicemesh
30:   - logging
31:   - database
32:   - anotherbackend
33: - name: backend-v2
34:   chart: charts/backend
35:   needs:
36:   - servicemesh
37:   - logging
38:   - database
39:   - anotherbackend
40: - name: anotherbackend
41:   chart: charts/anotherbackend
42:   needs:
43:   - servicemesh
44:   - logging
45:   - database
46: - name: servicemesh
47:   chart: charts/istio
48:   needs:
49:   - logging
50: - name: logging
51:   chart: charts/fluent-bit
52: - name: front-proxy
53:   chart: stable/envoy
54: 

merged environment: &{default map[] map[]}
10 release(s) found in helmfile.yaml

processing 5 groups of releases in this order:
GROUP RELEASES
1     frontend-v3, frontend-v2, frontend-v1
2     backend-v2, backend-v1
3     anotherbackend
4     database, servicemesh
5     front-proxy, logging

processing releases in group 1/5: frontend-v3, frontend-v2, frontend-v1
release "frontend-v3" processed
release "frontend-v2" processed
release "frontend-v1" processed
processing releases in group 2/5: backend-v2, backend-v1
release "backend-v2" processed
release "backend-v1" processed
processing releases in group 3/5: anotherbackend
release "anotherbackend" processed
processing releases in group 4/5: database, servicemesh
release "database" processed
release "servicemesh" processed
processing releases in group 5/5: front-proxy, logging
release "front-proxy" processed
release "logging" processed

DELETED RELEASES:
NAME
frontend-v3
frontend-v2
frontend-v1
backend-v2
backend-v1
anotherbackend
database
servicemesh
front-proxy
logging
`,
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(tc.name, func(t *testing.T) {
			wantUpgrades := tc.upgraded
			wantDeletes := tc.deleted

			var helm = &exectest.Helm{
				FailOnUnexpectedList: true,
				FailOnUnexpectedDiff: true,
				Lists:                tc.lists,
				Diffs:                tc.diffs,
				DiffMutex:            &sync.Mutex{},
				ChartsMutex:          &sync.Mutex{},
				ReleasesMutex:        &sync.Mutex{},
			}

			bs := &bytes.Buffer{}

			func() {
				logReader, logWriter := io.Pipe()

				logFlushed := &sync.WaitGroup{}
				// Ensure all the log is consumed into `bs` by calling `logWriter.Close()` followed by `logFlushed.Wait()`
				logFlushed.Add(1)
				go func() {
					scanner := bufio.NewScanner(logReader)
					for scanner.Scan() {
						bs.Write(scanner.Bytes())
						bs.WriteString("\n")
					}
					logFlushed.Done()
				}()

				defer func() {
					// This is here to avoid data-trace on bytes buffer `bs` to capture logs
					if err := logWriter.Close(); err != nil {
						panic(err)
					}
					logFlushed.Wait()
				}()

				logger := helmexec.NewLogger(logWriter, "debug")

				valsRuntime, err := vals.New(vals.Options{CacheSize: 32})
				if err != nil {
					t.Errorf("unexpected error creating vals runtime: %v", err)
				}

				app := appWithFs(&App{
					OverrideHelmBinary:  DefaultHelmBinary,
					glob:                filepath.Glob,
					abs:                 filepath.Abs,
					OverrideKubeContext: "default",
					Env:                 "default",
					Logger:              logger,
					helms: map[helmKey]helmexec.Interface{
						createHelmKey("helm", "default"): helm,
					},
					valsRuntime: valsRuntime,
				}, tc.files)

				if tc.ns != "" {
					app.Namespace = tc.ns
				}

				if tc.selectors != nil {
					app.Selectors = tc.selectors
				}

				destroyErr := app.Destroy(destroyConfig{
					// if we check log output, concurrency must be 1. otherwise the test becomes non-deterministic.
					concurrency: tc.concurrency,
					logger:      logger,
				})

				if tc.error == "" && destroyErr != nil {
					t.Fatalf("unexpected error for data defined at %s: %v", tc.loc, destroyErr)
				} else if tc.error != "" && destroyErr == nil {
					t.Fatalf("expected error did not occur for data defined at %s", tc.loc)
				} else if tc.error != "" && destroyErr != nil && tc.error != destroyErr.Error() {
					t.Fatalf("invalid error: expected %q, got %q", tc.error, destroyErr.Error())
				}

				if len(wantUpgrades) > len(helm.Releases) {
					t.Fatalf("insufficient number of upgrades: got %d, want %d", len(helm.Releases), len(wantUpgrades))
				}

				for relIdx := range wantUpgrades {
					if wantUpgrades[relIdx].Name != helm.Releases[relIdx].Name {
						t.Errorf("releases[%d].name: got %q, want %q", relIdx, helm.Releases[relIdx].Name, wantUpgrades[relIdx].Name)
					}
					for flagIdx := range wantUpgrades[relIdx].Flags {
						if wantUpgrades[relIdx].Flags[flagIdx] != helm.Releases[relIdx].Flags[flagIdx] {
							t.Errorf("releaes[%d].flags[%d]: got %v, want %v", relIdx, flagIdx, helm.Releases[relIdx].Flags[flagIdx], wantUpgrades[relIdx].Flags[flagIdx])
						}
					}
				}

				if len(wantDeletes) > len(helm.Deleted) {
					t.Fatalf("insufficient number of deletes: got %d, want %d", len(helm.Deleted), len(wantDeletes))
				}

				for relIdx := range wantDeletes {
					if wantDeletes[relIdx].Name != helm.Deleted[relIdx].Name {
						t.Errorf("releases[%d].name: got %q, want %q", relIdx, helm.Deleted[relIdx].Name, wantDeletes[relIdx].Name)
					}
					for flagIdx := range wantDeletes[relIdx].Flags {
						if wantDeletes[relIdx].Flags[flagIdx] != helm.Deleted[relIdx].Flags[flagIdx] {
							t.Errorf("releaes[%d].flags[%d]: got %v, want %v", relIdx, flagIdx, helm.Deleted[relIdx].Flags[flagIdx], wantDeletes[relIdx].Flags[flagIdx])
						}
					}
				}
			}()

			if tc.log != "" {
				actual := bs.String()

				diff, exists := testhelper.Diff(tc.log, actual, 3)
				if exists {
					t.Errorf("unexpected log for data defined %s:\nDIFF\n%s\nEOD", tc.loc, diff)
				}
			}
		})
	}
}
