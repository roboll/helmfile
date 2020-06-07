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

type diffConfig struct {
	args              string
	values            []string
	retainValuesFiles bool
	set               []string
	validate          bool
	skipDeps          bool
	includeTests      bool
	suppressSecrets   bool
	suppressDiff      bool
	noColor           bool
	context           int
	concurrency       int
	detailedExitcode  bool
	interactive       bool
	logger            *zap.SugaredLogger
}

func (a diffConfig) Args() string {
	return a.args
}

func (a diffConfig) Values() []string {
	return a.values
}

func (a diffConfig) Set() []string {
	return a.set
}

func (a diffConfig) Validate() bool {
	return a.validate
}

func (a diffConfig) SkipDeps() bool {
	return a.skipDeps
}

func (a diffConfig) IncludeTests() bool {
	return a.includeTests
}

func (a diffConfig) SuppressSecrets() bool {
	return a.suppressSecrets
}

func (a diffConfig) SuppressDiff() bool {
	return a.suppressDiff
}

func (a diffConfig) NoColor() bool {
	return a.noColor
}

func (a diffConfig) Context() int {
	return a.context
}

func (a diffConfig) Concurrency() int {
	return a.concurrency
}

func (a diffConfig) DetailedExitcode() bool {
	return a.detailedExitcode
}

func (a diffConfig) Interactive() bool {
	return a.interactive
}

func (a diffConfig) Logger() *zap.SugaredLogger {
	return a.logger
}

func (a diffConfig) RetainValuesFiles() bool {
	return a.retainValuesFiles
}

func TestDiff(t *testing.T) {
	testcases := []struct {
		name             string
		loc              string
		ns               string
		concurrency      int
		detailedExitcode bool
		error            string
		files            map[string]string
		selectors        []string
		lists            map[exectest.ListKey]string
		diffs            map[exectest.DiffKey]error
		upgraded         []exectest.Release
		deleted          []exectest.Release
		log              string
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
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				// noop on frontend-v2
				exectest.DiffKey{Name: "frontend-v2", Chart: "charts/frontend", Flags: "--kube-contextdefault--detailed-exitcode"}: nil,
				// install frontend-v3
				exectest.DiffKey{Name: "frontend-v3", Chart: "charts/frontend", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				// upgrades
				exectest.DiffKey{Name: "logging", Chart: "charts/fluent-bit", Flags: "--kube-contextdefault--detailed-exitcode"}:            helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "front-proxy", Chart: "stable/envoy", Flags: "--kube-contextdefault--detailed-exitcode"}:             helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "servicemesh", Chart: "charts/istio", Flags: "--kube-contextdefault--detailed-exitcode"}:             helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "database", Chart: "charts/mysql", Flags: "--kube-contextdefault--detailed-exitcode"}:                helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "backend-v2", Chart: "charts/backend", Flags: "--kube-contextdefault--detailed-exitcode"}:            helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "anotherbackend", Chart: "charts/anotherbackend", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				// delete frontend-v1 and backend-v1
				exectest.ListKey{Filter: "^frontend-v1$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
frontend-v1 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	backend-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^backend-v1$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
backend-v1 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	backend-3.1.0	3.1.0      	default
`,
			},
			// Disable concurrency to avoid in-deterministic result
			concurrency: 1,
			upgraded:    []exectest.Release{},
			deleted:     []exectest.Release{},
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

Affected releases are:
  anotherbackend (charts/anotherbackend) UPDATED
  backend-v1 (charts/backend) DELETED
  backend-v2 (charts/backend) UPDATED
  database (charts/mysql) UPDATED
  front-proxy (stable/envoy) UPDATED
  frontend-v1 (charts/frontend) DELETED
  frontend-v3 (charts/frontend) UPDATED
  logging (charts/fluent-bit) UPDATED
  servicemesh (charts/istio) UPDATED

`,
		},
		//
		// noop: no changes
		//
		{
			name: "noop",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
- name: foo
  chart: mychart1
  installed: false
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			error:            "",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: nil,
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: ``,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			upgraded: []exectest.Release{},
			deleted:  []exectest.Release{},
		},
		//
		// install
		//
		{
			name: "install",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: baz
  chart: mychart3
- name: foo
  chart: mychart1
  needs:
  - bar
- name: bar
  chart: mychart2
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "baz", Chart: "mychart3", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists:       map[exectest.ListKey]string{},
			upgraded:    []exectest.Release{},
			deleted:     []exectest.Release{},
			concurrency: 1,
			log: `processing file "helmfile.yaml" in directory "."
first-pass rendering starting for "helmfile.yaml.part.0": inherited=&{default map[] map[]}, overrode=<nil>
first-pass uses: &{default map[] map[]}
first-pass rendering output of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: baz
 3:   chart: mychart3
 4: - name: foo
 5:   chart: mychart1
 6:   needs:
 7:   - bar
 8: - name: bar
 9:   chart: mychart2
10: 

first-pass produced: &{default map[] map[]}
first-pass rendering result of "helmfile.yaml.part.0": {default map[] map[]}
vals:
map[]
defaultVals:[]
second-pass rendering result of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: baz
 3:   chart: mychart3
 4: - name: foo
 5:   chart: mychart1
 6:   needs:
 7:   - bar
 8: - name: bar
 9:   chart: mychart2
10: 

merged environment: &{default map[] map[]}
3 release(s) found in helmfile.yaml

Affected releases are:
  bar (mychart2) UPDATED
  baz (mychart3) UPDATED
  foo (mychart1) UPDATED

`,
		},
		//
		// upgrades
		//
		{
			name: "upgrade when foo needs bar",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
- name: foo
  chart: mychart1
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when bar needs foo",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: foo
  chart: mychart1
- name: bar
  chart: mychart2
  needs:
  - foo
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when foo needs bar, with ns override",
			loc:  location(),
			ns:   "testNamespace",
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
- name: foo
  chart: mychart1
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--namespacetestNamespace--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--namespacetestNamespace--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when bar needs foo, with ns override",
			loc:  location(),
			ns:   "testNamespace",
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: foo
  chart: mychart1
- name: bar
  chart: mychart2
  needs:
  - foo
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--namespacetestNamespace--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--namespacetestNamespace--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when ns1/foo needs ns2/bar",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: foo
  chart: mychart1
  namespace: ns1
  needs:
  - ns2/bar
- name: bar
  chart: mychart2
  namespace: ns2
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--namespacens2--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--namespacens1--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when ns2/bar needs ns1/foo",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
  namespace: ns2
  needs:
  - ns1/foo
- name: foo
  chart: mychart1
  namespace: ns1
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--namespacens2--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--namespacens1--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when tns1/ns1/foo needs tns2/ns2/bar",
			loc:  location(),

			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: foo
  chart: mychart1
  namespace: ns1
  tillerNamespace: tns1
  needs:
  - tns2/ns2/bar
- name: bar
  chart: mychart2
  namespace: ns2
  tillerNamespace: tns2
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--tiller-namespacetns2--kube-contextdefault--namespacens2--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--tiller-namespacetns1--kube-contextdefault--namespacens1--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
		},
		{
			name: "upgrade when tns2/ns2/bar needs tns1/ns1/foo",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
  namespace: ns2
  tillerNamespace: tns2
  needs:
  - tns1/ns1/foo
- name: foo
  chart: mychart1
  namespace: ns1
  tillerNamespace: tns1
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--tiller-namespacetns2--kube-contextdefault--namespacens2--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--tiller-namespacetns1--kube-contextdefault--namespacens1--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
			// as we check for log output, set concurrency to 1 to avoid non-deterministic test result
			concurrency: 1,
			log: `processing file "helmfile.yaml" in directory "."
first-pass rendering starting for "helmfile.yaml.part.0": inherited=&{default map[] map[]}, overrode=<nil>
first-pass uses: &{default map[] map[]}
first-pass rendering output of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: bar
 3:   chart: mychart2
 4:   namespace: ns2
 5:   tillerNamespace: tns2
 6:   needs:
 7:   - tns1/ns1/foo
 8: - name: foo
 9:   chart: mychart1
10:   namespace: ns1
11:   tillerNamespace: tns1
12: 

first-pass produced: &{default map[] map[]}
first-pass rendering result of "helmfile.yaml.part.0": {default map[] map[]}
vals:
map[]
defaultVals:[]
second-pass rendering result of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: bar
 3:   chart: mychart2
 4:   namespace: ns2
 5:   tillerNamespace: tns2
 6:   needs:
 7:   - tns1/ns1/foo
 8: - name: foo
 9:   chart: mychart1
10:   namespace: ns1
11:   tillerNamespace: tns1
12: 

merged environment: &{default map[] map[]}
2 release(s) found in helmfile.yaml

Affected releases are:
  bar (mychart2) UPDATED
  foo (mychart1) UPDATED

`,
		},
		//
		// deletes: deleting all releases in the correct order
		//
		{
			name: "delete foo and bar when foo needs bar",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
  installed: false
- name: foo
  chart: mychart1
  installed: false
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
foo 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart1-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			deleted: []exectest.Release{},
		},
		{
			name: "delete foo and bar when bar needs foo",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
  installed: false
  needs:
  - foo
- name: foo
  chart: mychart1
  installed: false
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
foo 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart1-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			deleted: []exectest.Release{},
		},
		//
		// upgrade and delete: upgrading one while deleting another
		//
		{
			name: "delete foo when foo needs bar",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
- name: foo
  chart: mychart1
  installed: false
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
foo 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart1-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			upgraded: []exectest.Release{},
			deleted:  []exectest.Release{},
		},
		{
			name: "delete bar when foo needs bar",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: bar
  chart: mychart2
  installed: false
- name: foo
  chart: mychart1
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
foo 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart1-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			upgraded: []exectest.Release{},
			deleted:  []exectest.Release{},
		},
		{
			name: "delete foo when bar needs foo",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: foo
  chart: mychart1
  installed: false
- name: bar
  chart: mychart2
  needs:
  - foo
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
foo 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart1-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			upgraded: []exectest.Release{},
			deleted:  []exectest.Release{},
		},
		{
			name: "delete bar when bar needs foo",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: foo
  chart: mychart1
- name: bar
  chart: mychart2
  installed: false
  needs:
  - foo
`,
			},
			detailedExitcode: true,
			error:            "Identified at least one change",
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "bar", Chart: "mychart2", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
			},
			lists: map[exectest.ListKey]string{
				exectest.ListKey{Filter: "^foo$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
foo 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart1-3.1.0	3.1.0      	default
`,
				exectest.ListKey{Filter: "^bar$", Flags: "--kube-contextdefault--deployed--failed--pending"}: `NAME	REVISION	UPDATED                 	STATUS  	CHART        	APP VERSION	NAMESPACE
bar 	4       	Fri Nov  1 08:40:07 2019	DEPLOYED	mychart2-3.1.0	3.1.0      	default
`,
			},
			upgraded: []exectest.Release{},
			deleted:  []exectest.Release{},
		},
		//
		// upgrades with selector
		//
		{
			// see https://github.com/roboll/helmfile/issues/919#issuecomment-549831747
			name: "upgrades with good selector",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
{{ $mark := "a" }}

releases:
- name: kubernetes-external-secrets
  chart: incubator/raw
  namespace: kube-system

- name: external-secrets
  chart: incubator/raw
  namespace: default
  labels:
    app: test
  needs:
  - kube-system/kubernetes-external-secrets

- name: my-release
  chart: incubator/raw
  namespace: default
  labels:
    app: test
  needs:
  - default/external-secrets
`,
			},
			selectors:        []string{"app=test"},
			detailedExitcode: true,
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "external-secrets", Chart: "incubator/raw", Flags: "--kube-contextdefault--namespacedefault--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "my-release", Chart: "incubator/raw", Flags: "--kube-contextdefault--namespacedefault--detailed-exitcode"}:       helmexec.ExitError{Code: 2},
			},
			upgraded: []exectest.Release{},
			// as we check for log output, set concurrency to 1 to avoid non-deterministic test result
			concurrency: 1,
			error:       "Identified at least one change",
			log: `processing file "helmfile.yaml" in directory "."
first-pass rendering starting for "helmfile.yaml.part.0": inherited=&{default map[] map[]}, overrode=<nil>
first-pass uses: &{default map[] map[]}
first-pass rendering output of "helmfile.yaml.part.0":
 0: 
 1: 
 2: 
 3: releases:
 4: - name: kubernetes-external-secrets
 5:   chart: incubator/raw
 6:   namespace: kube-system
 7: 
 8: - name: external-secrets
 9:   chart: incubator/raw
10:   namespace: default
11:   labels:
12:     app: test
13:   needs:
14:   - kube-system/kubernetes-external-secrets
15: 
16: - name: my-release
17:   chart: incubator/raw
18:   namespace: default
19:   labels:
20:     app: test
21:   needs:
22:   - default/external-secrets
23: 

first-pass produced: &{default map[] map[]}
first-pass rendering result of "helmfile.yaml.part.0": {default map[] map[]}
vals:
map[]
defaultVals:[]
second-pass rendering result of "helmfile.yaml.part.0":
 0: 
 1: 
 2: 
 3: releases:
 4: - name: kubernetes-external-secrets
 5:   chart: incubator/raw
 6:   namespace: kube-system
 7: 
 8: - name: external-secrets
 9:   chart: incubator/raw
10:   namespace: default
11:   labels:
12:     app: test
13:   needs:
14:   - kube-system/kubernetes-external-secrets
15: 
16: - name: my-release
17:   chart: incubator/raw
18:   namespace: default
19:   labels:
20:     app: test
21:   needs:
22:   - default/external-secrets
23: 

merged environment: &{default map[] map[]}
2 release(s) matching app=test found in helmfile.yaml

Affected releases are:
  external-secrets (incubator/raw) UPDATED
  my-release (incubator/raw) UPDATED

`,
		},
		{
			// see https://github.com/roboll/helmfile/issues/919#issuecomment-549831747
			name: "upgrades with bad selector",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
{{ $mark := "a" }}

releases:
- name: kubernetes-external-secrets
  chart: incubator/raw
  namespace: kube-system

- name: external-secrets
  chart: incubator/raw
  namespace: default
  labels:
    app: test
  needs:
  - kube-system/kubernetes-external-secrets

- name: my-release
  chart: incubator/raw
  namespace: default
  labels:
    app: test
  needs:
  - default/external-secrets
`,
			},
			selectors:        []string{"app=test_non_existent"},
			detailedExitcode: true,
			diffs:            map[exectest.DiffKey]error{},
			upgraded:         []exectest.Release{},
			error:            "err: no releases found that matches specified selector(app=test_non_existent) and environment(default), in any helmfile",
			// as we check for log output, set concurrency to 1 to avoid non-deterministic test result
			concurrency: 1,
			log: `processing file "helmfile.yaml" in directory "."
first-pass rendering starting for "helmfile.yaml.part.0": inherited=&{default map[] map[]}, overrode=<nil>
first-pass uses: &{default map[] map[]}
first-pass rendering output of "helmfile.yaml.part.0":
 0: 
 1: 
 2: 
 3: releases:
 4: - name: kubernetes-external-secrets
 5:   chart: incubator/raw
 6:   namespace: kube-system
 7: 
 8: - name: external-secrets
 9:   chart: incubator/raw
10:   namespace: default
11:   labels:
12:     app: test
13:   needs:
14:   - kube-system/kubernetes-external-secrets
15: 
16: - name: my-release
17:   chart: incubator/raw
18:   namespace: default
19:   labels:
20:     app: test
21:   needs:
22:   - default/external-secrets
23: 

first-pass produced: &{default map[] map[]}
first-pass rendering result of "helmfile.yaml.part.0": {default map[] map[]}
vals:
map[]
defaultVals:[]
second-pass rendering result of "helmfile.yaml.part.0":
 0: 
 1: 
 2: 
 3: releases:
 4: - name: kubernetes-external-secrets
 5:   chart: incubator/raw
 6:   namespace: kube-system
 7: 
 8: - name: external-secrets
 9:   chart: incubator/raw
10:   namespace: default
11:   labels:
12:     app: test
13:   needs:
14:   - kube-system/kubernetes-external-secrets
15: 
16: - name: my-release
17:   chart: incubator/raw
18:   namespace: default
19:   labels:
20:     app: test
21:   needs:
22:   - default/external-secrets
23: 

merged environment: &{default map[] map[]}
0 release(s) matching app=test_non_existent found in helmfile.yaml

`,
		},
		//
		// error cases
		//
		{
			name: "non-existent release in needs",
			loc:  location(),
			files: map[string]string{
				"/path/to/helmfile.yaml": `
releases:
- name: baz
  namespace: ns1
  chart: mychart3
- name: foo
  chart: mychart1
  needs:
  - bar
`,
			},
			detailedExitcode: true,
			diffs: map[exectest.DiffKey]error{
				exectest.DiffKey{Name: "baz", Chart: "mychart3", Flags: "--kube-contextdefault--namespacens1--detailed-exitcode"}: helmexec.ExitError{Code: 2},
				exectest.DiffKey{Name: "foo", Chart: "mychart1", Flags: "--kube-contextdefault--detailed-exitcode"}:               helmexec.ExitError{Code: 2},
			},
			lists:       map[exectest.ListKey]string{},
			upgraded:    []exectest.Release{},
			deleted:     []exectest.Release{},
			concurrency: 1,
			error:       `in ./helmfile.yaml: "foo" depends on nonexistent release "bar"`,
			log: `processing file "helmfile.yaml" in directory "."
first-pass rendering starting for "helmfile.yaml.part.0": inherited=&{default map[] map[]}, overrode=<nil>
first-pass uses: &{default map[] map[]}
first-pass rendering output of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: baz
 3:   namespace: ns1
 4:   chart: mychart3
 5: - name: foo
 6:   chart: mychart1
 7:   needs:
 8:   - bar
 9: 

first-pass produced: &{default map[] map[]}
first-pass rendering result of "helmfile.yaml.part.0": {default map[] map[]}
vals:
map[]
defaultVals:[]
second-pass rendering result of "helmfile.yaml.part.0":
 0: 
 1: releases:
 2: - name: baz
 3:   namespace: ns1
 4:   chart: mychart3
 5: - name: foo
 6:   chart: mychart1
 7:   needs:
 8:   - bar
 9: 

merged environment: &{default map[] map[]}
2 release(s) found in helmfile.yaml

err: "foo" depends on nonexistent release "bar"
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

				diffErr := app.Diff(diffConfig{
					// if we check log output, concurrency must be 1. otherwise the test becomes non-deterministic.
					concurrency:      tc.concurrency,
					logger:           logger,
					detailedExitcode: tc.detailedExitcode,
				})
				if tc.error == "" && diffErr != nil {
					t.Fatalf("unexpected error for data defined at %s: %v", tc.loc, diffErr)
				} else if tc.error != "" && diffErr == nil {
					t.Fatalf("expected error did not occur for data defined at %s", tc.loc)
				} else if tc.error != "" && diffErr != nil && tc.error != diffErr.Error() {
					t.Fatalf("invalid error: expected %q, got %q", tc.error, diffErr.Error())
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
