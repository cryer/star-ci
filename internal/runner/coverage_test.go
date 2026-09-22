package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryer/star-ci/internal/plan"
)

func f64(v float64) *float64 { return &v }

func writeReport(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseCoverageSummaryJSON(t *testing.T) {
	pct, err := parseCoverageSummaryJSON([]byte(`{"total":{"lines":{"pct":82.5}},"src/a.js":{"lines":{"pct":80}}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pct != 82.5 {
		t.Errorf("pct = %v, want 82.5", pct)
	}
	for name, data := range map[string]string{
		"malformed":     `{"total":`,
		"missing total": `{"src/a.js":{"lines":{"pct":80}}}`,
	} {
		if _, err := parseCoverageSummaryJSON([]byte(data)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseCoverageXML(t *testing.T) {
	pct, err := parseCoverageXML([]byte(`<?xml version="1.0"?><coverage line-rate="0.75" branch-rate="0.5"><packages/></coverage>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pct != 75 {
		t.Errorf("pct = %v, want 75", pct)
	}
	for name, data := range map[string]string{
		"malformed":         `<coverage line-rate=`,
		"wrong root":        `<report line-rate="0.5"></report>`,
		"missing line-rate": `<coverage></coverage>`,
		"bad line-rate":     `<coverage line-rate="high"></coverage>`,
	} {
		if _, err := parseCoverageXML([]byte(data)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseCoverprofile(t *testing.T) {
	profile := "mode: set\nexample.com/a.go:3.10,5.2 2 1\nexample.com/b.go:8.1,10.9 2 0\n"
	pct, err := parseCoverprofile([]byte(profile))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pct != 50 {
		t.Errorf("pct = %v, want 50", pct)
	}
	for name, data := range map[string]string{
		"missing mode line": "example.com/a.go:3.10,5.2 2 1\n",
		"no statements":     "mode: set\n",
		"malformed block":   "mode: set\ngarbage\n",
		"bad statement num": "mode: set\nexample.com/a.go:3.10,5.2 x 1\n",
		"bad execution num": "mode: set\nexample.com/a.go:3.10,5.2 2 x\n",
		"zero statements":   "mode: set\nexample.com/a.go:3.10,5.2 0 1\n",
	} {
		if _, err := parseCoverprofile([]byte(data)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestMeasureCoverageLowestWins(t *testing.T) {
	root := t.TempDir()
	writeReport(t, root, "coverage/coverage-summary.json", `{"total":{"lines":{"pct":90}}}`)
	writeReport(t, root, "coverage.xml", `<coverage line-rate="0.60"></coverage>`)
	pct, source, found, err := measureCoverage(root)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if pct != 60 || source != "coverage.xml" {
		t.Errorf("pct/source = %v/%q, want lowest 60 from coverage.xml", pct, source)
	}
}

func TestMeasureCoverageNoReport(t *testing.T) {
	if _, _, found, err := measureCoverage(t.TempDir()); err != nil || found {
		t.Errorf("empty dir: found=%v err=%v, want false/nil", found, err)
	}
}

func TestCheckCoverageNoReportSkips(t *testing.T) {
	var buf bytes.Buffer
	res, err := checkCoverage(t.TempDir(), 80, &buf)
	if err != nil || res != nil {
		t.Errorf("res=%+v err=%v, want nil/nil", res, err)
	}
	if !strings.Contains(buf.String(), "no known report found, skipping") {
		t.Errorf("expected skip notice, got:\n%s", buf.String())
	}
}

func TestCheckCoverageBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		report    string
		threshold float64
		wantErr   bool
	}{
		{"below threshold", `{"total":{"lines":{"pct":79.9}}}`, 80, true},
		{"equal passes", `{"total":{"lines":{"pct":80}}}`, 80, false},
		{"above threshold", `{"total":{"lines":{"pct":95.5}}}`, 80, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeReport(t, root, "coverage/coverage-summary.json", tc.report)
			var buf bytes.Buffer
			res, err := checkCoverage(root, tc.threshold, &buf)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v\noutput:\n%s", err, tc.wantErr, buf.String())
			}
			if res == nil {
				t.Fatal("expected a result when a report exists")
			}
			if res.Met() == tc.wantErr {
				t.Errorf("Met() = %v, inconsistent with err %v", res.Met(), err)
			}
		})
	}
}

func TestCheckCoverageMalformedReportFails(t *testing.T) {
	root := t.TempDir()
	writeReport(t, root, "cover.out", "garbage\n")
	var buf bytes.Buffer
	if _, err := checkCoverage(root, 80, &buf); err == nil {
		t.Fatal("malformed report should fail the check")
	} else if !strings.Contains(err.Error(), "cover.out") {
		t.Errorf("error should name the report file, got: %v", err)
	}
}

func TestRunCoverageEndToEnd(t *testing.T) {
	requireSh(t)
	cases := []struct {
		name      string
		pct       string
		threshold float64
		wantErr   bool
	}{
		{"pass", "90", 80, false},
		{"fail", "70", 80, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pl := testPlan(plan.Step{
				ID: "node-test", Name: "Run tests", Category: plan.CatTest, Reason: "test",
				Commands: []string{
					"mkdir -p coverage && echo '{\"total\":{\"lines\":{\"pct\":" + tc.pct + "}}}' > coverage/coverage-summary.json",
				},
			})
			var buf bytes.Buffer
			err := Run(context.Background(), t.TempDir(), pl, &buf, Options{CoverageThreshold: f64(tc.threshold)})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v\noutput:\n%s", err, tc.wantErr, buf.String())
			}
			if tc.wantErr && !strings.Contains(err.Error(), "below declared threshold 80.0%") {
				t.Errorf("error should state threshold, got: %v", err)
			}
			if !strings.Contains(buf.String(), "star-ci: coverage:") {
				t.Errorf("output missing coverage line:\n%s", buf.String())
			}
		})
	}
}

func TestRunCoverageNoThresholdIsNoop(t *testing.T) {
	requireSh(t)
	pl := testPlan(plan.Step{
		ID: "node-test", Name: "Run tests", Category: plan.CatTest, Reason: "test",
		Commands: []string{
			"mkdir -p coverage && echo '{\"total\":{\"lines\":{\"pct\":1}}}' > coverage/coverage-summary.json",
		},
	})
	var buf bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), pl, &buf, Options{}); err != nil {
		t.Fatalf("no threshold declared: run must not check coverage, got: %v", err)
	}
	if strings.Contains(buf.String(), "coverage:") {
		t.Errorf("no coverage output expected without a threshold:\n%s", buf.String())
	}
}
