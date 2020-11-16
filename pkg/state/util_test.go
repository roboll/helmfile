package state

import "testing"

func TestIsLocalChart(t *testing.T) {
	testcases := []struct {
		input    string
		expected bool
	}{
		{
			input:    "mychart",
			expected: true,
		},
		{
			input:    "stable/mysql",
			expected: false,
		},
		{
			input:    "center/stable/mysql",
			expected: false,
		},
		{
			input:    "./charts/myapp",
			expected: true,
		},
		{
			input:    "center/stable/myapp",
			expected: false,
		},
		{
			input:    "./charts/mysubsystem/myapp",
			expected: true,
		},
		{
			input:    "/charts/mysubsystem/myapp",
			expected: true,
		},
		{
			// Regression test case for:
			// * https://github.com/roboll/helmfile/issues/675
			// * https://github.com/roboll/helmfile/issues/687
			input:    "https://github.com/arangodb/kube-arangodb/releases/download/0.3.11/kube-arangodb-crd.tgz",
			expected: false,
		},
		{
			input:    "https://example.com/bar.tgz",
			expected: false,
		},
	}

	for i := range testcases {
		testcase := testcases[i]

		actual := isLocalChart(testcase.input)

		if testcase.expected != actual {
			t.Errorf("unexpected result: isLocalChart(\"%s\"): expected=%v, got=%v", testcase.input, testcase.expected, actual)
		}
	}
}

func TestResolveRemortChart(t *testing.T) {
	testcases := []struct {
		input  string
		repo   string
		chart  string
		remote bool
	}{
		{
			input:  "mychart",
			remote: false,
		},
		{
			input:  "stable/mysql",
			repo:   "stable",
			chart:  "mysql",
			remote: true,
		},
		{
			input:  "./charts/myapp",
			remote: false,
		},
		{
			input:  "center/stable/myapp",
			repo:   "center",
			chart:  "stable/myapp",
			remote: true,
		},
		{
			input:  "./charts/mysubsystem/myapp",
			remote: false,
		},
		{
			input:  "/charts/mysubsystem/myapp",
			remote: false,
		},
		{
			// Regression test case for:
			// * https://github.com/roboll/helmfile/issues/675
			// * https://github.com/roboll/helmfile/issues/687
			input:  "https://github.com/arangodb/kube-arangodb/releases/download/0.3.11/kube-arangodb-crd.tgz",
			remote: false,
		},
		{
			input:  "https://example.com/bar.tgz",
			remote: false,
		},
	}

	for i := range testcases {
		testcase := testcases[i]

		repo, chart, actual := resolveRemoteChart(testcase.input)

		if testcase.remote != actual {
			t.Fatalf("unexpected result: reolveRemoteChart(\"%s\"): expected=%v, got=%v", testcase.input, testcase.remote, actual)
		}

		if testcase.repo != repo {
			t.Errorf("unexpected repo: %s: expected=%v, got=%v", testcase.input, testcase.repo, repo)
		}

		if testcase.chart != chart {
			t.Errorf("unexpected chart: %s: expected=%v, got=%v", testcase.input, testcase.chart, chart)
		}
	}
}

func TestNormalizeChart(t *testing.T) {
	testcases := []struct {
		input    string
		expected string
	}{
		{
			input:    "mychart",
			expected: "/path/to/mychart",
		},
		{
			input:    "/charts/mychart",
			expected: "/charts/mychart",
		},
	}

	for i := range testcases {
		testcase := testcases[i]

		actual := normalizeChart("/path/to", testcase.input)

		if testcase.expected != actual {
			t.Fatalf("unexpected result: normalizeChart(\"/path/to\", \"%s\"): expected=%v, got=%v", testcase.input, testcase.expected, actual)
		}
	}
}
