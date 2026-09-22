package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CoverageResult records the outcome of a coverage threshold check.
type CoverageResult struct {
	Percent   float64 // measured total line coverage
	Threshold float64 // declared threshold from .star-ci.yml
	Source    string  // report file the percentage came from, relative to root
}

// Met reports whether the measured coverage satisfies the threshold.
// Equality passes.
func (c *CoverageResult) Met() bool { return c.Percent >= c.Threshold }

// coverageReports lists known report locations in a fixed probe order.
// Every report that exists is parsed; when several are present, the lowest
// percentage wins (most conservative), so a multi-language repo must satisfy
// the threshold in its weakest measured ecosystem.
var coverageReports = []struct {
	path  string // slash-separated, relative to the repo root
	parse func([]byte) (float64, error)
}{
	{"coverage/coverage-summary.json", parseCoverageSummaryJSON}, // vitest / jest
	{"coverage.xml", parseCoverageXML},                           // pytest-cov / coverage.py
	{"cover.out", parseCoverprofile},                             // go test -coverprofile
	{"coverage.out", parseCoverprofile},
}

// checkCoverage enforces the declared line-coverage threshold after all steps
// passed. With no known report on disk it is a no-op (prints a skip notice):
// the threshold only applies when tests actually emitted coverage. A report
// that exists but cannot be parsed is an error — the declared standard cannot
// be verified.
func checkCoverage(root string, threshold float64, w io.Writer) (*CoverageResult, error) {
	pct, source, found, err := measureCoverage(root)
	if err != nil {
		return nil, fmt.Errorf("coverage: %v", err)
	}
	if !found {
		fmt.Fprintf(w, "star-ci: coverage: threshold %.1f%% declared but no known report found, skipping check\n", threshold)
		return nil, nil
	}
	res := &CoverageResult{Percent: pct, Threshold: threshold, Source: source}
	if !res.Met() {
		fmt.Fprintf(w, "star-ci: coverage: %.1f%% (from %s) is below threshold %.1f%%\n",
			pct, source, threshold)
		return res, fmt.Errorf("coverage %.1f%% (from %s) below declared threshold %.1f%%",
			pct, source, threshold)
	}
	fmt.Fprintf(w, "star-ci: coverage: %.1f%% (from %s) meets threshold %.1f%%\n",
		pct, source, threshold)
	return res, nil
}

// measureCoverage parses every known report found under root and returns the
// lowest percentage among them.
func measureCoverage(root string) (pct float64, source string, found bool, err error) {
	best := math.Inf(1)
	for _, r := range coverageReports {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(r.path)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, "", false, err
		}
		p, err := r.parse(data)
		if err != nil {
			return 0, "", false, fmt.Errorf("%s: %v", r.path, err)
		}
		if p < best {
			best, source, found = p, r.path, true
		}
	}
	if !found {
		return 0, "", false, nil
	}
	return best, source, true, nil
}

// parseCoverageSummaryJSON reads the vitest/jest summary format and returns
// total.lines.pct.
func parseCoverageSummaryJSON(data []byte) (float64, error) {
	var v struct {
		Total *struct {
			Lines struct {
				Pct float64 `json:"pct"`
			} `json:"lines"`
		} `json:"total"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, fmt.Errorf("malformed coverage summary: %v", err)
	}
	if v.Total == nil {
		return 0, fmt.Errorf("coverage summary has no \"total\" section")
	}
	return v.Total.Lines.Pct, nil
}

// parseCoverageXML reads the coverage.py XML format and returns the root
// element's line-rate attribute as a percentage.
func parseCoverageXML(data []byte) (float64, error) {
	var v struct {
		XMLName  xml.Name `xml:"coverage"`
		LineRate string   `xml:"line-rate,attr"`
	}
	if err := xml.Unmarshal(data, &v); err != nil {
		return 0, fmt.Errorf("malformed coverage XML: %v", err)
	}
	if v.LineRate == "" {
		return 0, fmt.Errorf("coverage XML root has no line-rate attribute")
	}
	rate, err := strconv.ParseFloat(v.LineRate, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid line-rate %q: not a number", v.LineRate)
	}
	return rate * 100, nil
}

// parseCoverprofile reads the go test -coverprofile block format:
//
//	mode: set
//	example.com/pkg/a.go:3.10,5.2 2 1
//
// and returns covered statements (count > 0) / total statements * 100.
func parseCoverprofile(data []byte) (float64, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "mode:") {
		return 0, fmt.Errorf("coverprofile must start with a 'mode:' line")
	}
	var total, covered int
	for i, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, fmt.Errorf("coverprofile line %d: want 'file:range numStmt count', got %q", i+2, line)
		}
		stmts, err := strconv.Atoi(fields[1])
		if err != nil || stmts <= 0 {
			return 0, fmt.Errorf("coverprofile line %d: invalid statement count %q", i+2, fields[1])
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, fmt.Errorf("coverprofile line %d: invalid execution count %q", i+2, fields[2])
		}
		total += stmts
		if count > 0 {
			covered += stmts
		}
	}
	if total == 0 {
		return 0, fmt.Errorf("coverprofile contains no statement blocks")
	}
	return float64(covered) / float64(total) * 100, nil
}
