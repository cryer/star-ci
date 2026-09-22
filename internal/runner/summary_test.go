package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
)

func TestDigestShort(t *testing.T) {
	out := "line1\nline2\n"
	if got := Digest(out, DigestLines); got != "line1\nline2" {
		t.Errorf("short output should pass through trimmed, got %q", got)
	}
}

func TestDigestTruncates(t *testing.T) {
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, "line"+strconv.Itoa(i))
	}
	got := Digest(strings.Join(lines, "\n"), 20)
	if !strings.HasPrefix(got, "... (10 earlier lines omitted)\nline11\n") {
		t.Errorf("digest should keep last 20 lines with omission marker, got:\n%s", got)
	}
	if strings.Contains(got, "line10") {
		t.Errorf("digest should drop lines before the tail window, got:\n%s", got)
	}
}

func TestDigestEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "\n\n"} {
		if got := Digest(in, DigestLines); got != "" {
			t.Errorf("Digest(%q) = %q, want empty", in, got)
		}
	}
}

func summaryTestPlan() plan.Plan {
	return plan.Plan{
		Profile: profile.Profile{
			Languages:      []profile.Language{{Name: "go", VersionHint: "1.22", Confidence: 0.9}},
			PackageManager: "go",
			TestRunner:     "go",
			Linters:        []string{"golangci-lint"},
		},
		Steps: []plan.Step{
			{ID: "go-test", Name: "Run tests", Category: plan.CatTest, Commands: []string{"go test ./..."}, Reason: "go.mod present"},
			{ID: "go-sec", Name: "Security scan", Category: plan.CatSecurity, Reason: "security sweep", Optional: true},
		},
	}
}

func TestRenderSummary(t *testing.T) {
	pl := summaryTestPlan()
	results := []StepResult{
		{Step: pl.Steps[0], Status: StatusFailed, Output: "ok pkg/a\nFAIL pkg/b\n"},
		{Step: pl.Steps[1], Status: StatusSkipped},
	}
	md := RenderSummary(pl, results, nil)
	for _, want := range []string{
		"## star-ci summary",
		"**Detected:** go 1.22",
		"- **Package manager:** go",
		"- **Linters:** golangci-lint",
		"| Step | Category | Status | Reason |",
		"| Run tests | test | failed | go.mod present |",
		"| Security scan | security | skipped | security sweep |",
		"### Failure digests",
		"<summary>Run tests (go-test) — failed</summary>",
		"```\nok pkg/a\nFAIL pkg/b\n```",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("summary missing %q\nsummary:\n%s", want, md)
		}
	}
	// Skipped steps have no output, so only one digest block.
	if strings.Count(md, "<details>") != 1 {
		t.Errorf("expected exactly one digest block\nsummary:\n%s", md)
	}
}

func TestRenderSummaryWarnedDigest(t *testing.T) {
	pl := summaryTestPlan()
	results := []StepResult{
		{Step: pl.Steps[0], Status: StatusPassed, Output: "all good\n"},
		{Step: pl.Steps[1], Status: StatusWarned, Output: "vuln found\n"},
	}
	md := RenderSummary(pl, results, nil)
	if !strings.Contains(md, "<summary>Security scan (go-sec) — warned</summary>") {
		t.Errorf("warned step with output should get a digest\nsummary:\n%s", md)
	}
	if strings.Contains(md, "all good") {
		t.Errorf("passed steps should not get digests\nsummary:\n%s", md)
	}
}

func TestRenderSummaryEscapesTableCells(t *testing.T) {
	pl := plan.Plan{Steps: []plan.Step{{ID: "x", Name: "A|B", Category: plan.CatTest, Reason: "a|b\nc"}}}
	md := RenderSummary(pl, []StepResult{{Step: pl.Steps[0], Status: StatusPassed}}, nil)
	if !strings.Contains(md, "| A\\|B | test | passed | a\\|b c |") {
		t.Errorf("pipes and newlines should be escaped in table cells\nsummary:\n%s", md)
	}
}

func TestRunWritesStepSummary(t *testing.T) {
	requireSh(t)
	summaryPath := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
	pl := testPlan(
		plan.Step{ID: "ok", Name: "OK", Category: plan.CatTest, Commands: []string{"echo fine"}, Reason: "test"},
		plan.Step{ID: "bad", Name: "Bad", Category: plan.CatTest, Commands: []string{"echo boom-output && exit 1"}, Reason: "test"},
		plan.Step{ID: "never", Name: "Never", Category: plan.CatBuild, Commands: []string{"echo unreached"}, Reason: "test"},
	)
	var buf bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), pl, &buf, Options{}); err == nil {
		t.Fatal("expected error from failing required step")
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("summary file should exist: %v", err)
	}
	md := string(data)
	for _, want := range []string{
		"| OK | test | passed | test |",
		"| Bad | test | failed | test |",
		"| Never | build | skipped | test |",
		"### Failure digests",
		"boom-output",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("summary file missing %q\nsummary:\n%s", want, md)
		}
	}
}

func TestRunSummaryAppends(t *testing.T) {
	requireSh(t)
	summaryPath := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
	pl := testPlan(plan.Step{ID: "ok", Name: "OK", Category: plan.CatTest, Commands: []string{"true"}, Reason: "test"})
	for i := 0; i < 2; i++ {
		var buf bytes.Buffer
		if err := Run(context.Background(), t.TempDir(), pl, &buf, Options{}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "## star-ci summary") != 2 {
		t.Errorf("summary should append across runs, got:\n%s", data)
	}
}

func TestRunWithoutSummaryEnvUnchanged(t *testing.T) {
	requireSh(t)
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	pl := testPlan(plan.Step{ID: "ok", Name: "OK", Category: plan.CatTest, Commands: []string{"echo hi"}, Reason: "test"})
	var buf bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), pl, &buf, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "star-ci summary") {
		t.Errorf("no summary content should leak into local output:\n%s", buf.String())
	}
}

func TestRunErrorCarriesDigest(t *testing.T) {
	requireSh(t)
	pl := testPlan(plan.Step{
		ID: "bad", Name: "Bad", Category: plan.CatTest,
		Commands: []string{"echo tail-marker && exit 1"}, Reason: "test",
	})
	var buf bytes.Buffer
	err := Run(context.Background(), t.TempDir(), pl, &buf, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "step bad (Bad) failed") {
		t.Errorf("error should keep the original contract prefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), "failure digest:\ntail-marker") {
		t.Errorf("error should carry the failure digest, got: %v", err)
	}
}
