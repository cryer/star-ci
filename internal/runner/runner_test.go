package runner

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/star-ci/star-ci/internal/plan"
	"github.com/star-ci/star-ci/internal/profile"
)

func requireSh(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found in PATH")
	}
}

func testPlan(steps ...plan.Step) plan.Plan {
	return plan.Plan{
		Steps: steps,
		Profile: profile.Profile{
			Languages: []profile.Language{{Name: "go", VersionHint: "1.22", Confidence: 0.9}},
		},
	}
}

func TestRunSuccess(t *testing.T) {
	requireSh(t)
	pl := testPlan(
		plan.Step{ID: "one", Name: "First", Category: plan.CatTest, Commands: []string{"echo hello"}, Reason: "test"},
		plan.Step{ID: "two", Name: "Second", Category: plan.CatTest, Commands: []string{"echo world", "echo again"}, Reason: "test"},
	)
	var buf bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), pl, &buf); err != nil {
		t.Fatalf("Run returned error: %v\noutput:\n%s", err, buf.String())
	}
	out := buf.String()
	for _, want := range []string{
		"star-ci: detected go 1.22, 2 steps",
		"==> First (test)",
		"    reason: test",
		"    $ echo hello",
		"hello",
		"world",
		"again",
		"star-ci: 2 steps passed, 0 optional warnings",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestRunFailFast(t *testing.T) {
	requireSh(t)
	pl := testPlan(
		plan.Step{ID: "ok", Name: "OK", Category: plan.CatTest, Commands: []string{"echo fine"}, Reason: "test"},
		plan.Step{ID: "bad", Name: "Bad", Category: plan.CatTest, Commands: []string{"exit 1"}, Reason: "test"},
		plan.Step{ID: "never", Name: "Never", Category: plan.CatTest, Commands: []string{"echo unreached"}, Reason: "test"},
	)
	var buf bytes.Buffer
	err := Run(context.Background(), t.TempDir(), pl, &buf)
	if err == nil {
		t.Fatal("expected error from failing required step")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention failed step id, got: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "==> Never") {
		t.Errorf("fail-fast violated: steps after failure executed\noutput:\n%s", out)
	}
	if !strings.Contains(out, "failure: step bad failed") {
		t.Errorf("output missing failure notice\noutput:\n%s", out)
	}
}

func TestRunOptionalWarns(t *testing.T) {
	requireSh(t)
	pl := testPlan(
		plan.Step{ID: "ok", Name: "OK", Category: plan.CatTest, Commands: []string{"echo fine"}, Reason: "test"},
		plan.Step{ID: "flaky", Name: "Flaky", Category: plan.CatSecurity, Commands: []string{"exit 3"}, Reason: "test", Optional: true},
		plan.Step{ID: "after", Name: "After", Category: plan.CatBuild, Commands: []string{"echo done"}, Reason: "test"},
	)
	var buf bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), pl, &buf); err != nil {
		t.Fatalf("optional failure should not error, got: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"warning: optional step flaky failed",
		"==> After (build)",
		"done",
		"star-ci: 2 steps passed, 1 optional warnings",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestRunEmptyPlan(t *testing.T) {
	var buf bytes.Buffer
	if err := Run(context.Background(), t.TempDir(), plan.Plan{}, &buf); err != nil {
		t.Fatalf("empty plan should succeed, got: %v", err)
	}
	if !strings.Contains(buf.String(), "0 steps passed, 0 optional warnings") {
		t.Errorf("unexpected output:\n%s", buf.String())
	}
}

func TestExplain(t *testing.T) {
	pl := testPlan(
		plan.Step{ID: "one", Name: "First", Category: plan.CatInstall, Commands: []string{"echo SENTINEL"}, Reason: "because"},
	)
	var buf bytes.Buffer
	Explain(pl, &buf)
	out := buf.String()
	for _, want := range []string{
		"star-ci: detected go 1.22, 1 steps",
		"==> First (install)",
		"    reason: because",
		"    $ echo SENTINEL",
		"dry run",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\noutput:\n%s", want, out)
		}
	}
	// Explain must not execute: SENTINEL appears only in the "$ echo" line.
	if strings.Count(out, "SENTINEL") != 1 {
		t.Errorf("Explain appears to have executed commands\noutput:\n%s", out)
	}
}
