package render

import (
	"strings"
	"testing"

	"github.com/star-ci/star-ci/internal/plan"
	"github.com/star-ci/star-ci/internal/profile"
)

func TestWorkflowYAML(t *testing.T) {
	prof := profile.Profile{PackageManager: "pnpm"}
	prof.AddLanguage("node", "20", 0.9)
	prof.AddLanguage("go", "1.22", 0.8)

	pl := plan.Plan{
		Profile: prof,
		Steps: []plan.Step{
			{ID: "node-install", Name: "Install dependencies", Category: plan.CatInstall,
				Commands: []string{"pnpm install"}},
			{ID: "node-test", Name: "Run tests", Category: plan.CatTest,
				Commands: []string{"pnpm test", "pnpm coverage"}},
			{ID: "security-audit", Name: "Audit", Category: plan.CatSecurity,
				Commands: []string{"pnpm audit"}, Optional: true},
		},
	}

	data, err := WorkflowYAML(pl)
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)

	for _, want := range []string{
		"name: star-ci",
		"on: [push, pull_request]",
		"    runs-on: ubuntu-latest",
		"      - uses: actions/checkout@v4",
		"uses: pnpm/action-setup@v4",
		"uses: actions/setup-node@v4",
		"          node-version: '20'",
		"          cache: pnpm",
		"uses: actions/setup-go@v5",
		"          go-version: '1.22'",
		"      - name: Install dependencies",
		"      - name: Run tests",
		"        continue-on-error: true\n        run: |\n          pnpm audit",
		"          pnpm test\n          pnpm coverage",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	// every plan step's "- name:" is followed later by a "run:" block
	for _, s := range pl.Steps {
		idx := strings.Index(y, "- name: "+s.Name)
		if idx < 0 {
			t.Fatalf("step %q not rendered", s.Name)
		}
		runIdx := strings.Index(y[idx:], "run: |")
		if runIdx < 0 {
			t.Errorf("no run block after step %q", s.Name)
		}
	}

	// pnpm setup must precede setup-node so the cache works
	if strings.Index(y, "pnpm/action-setup") > strings.Index(y, "actions/setup-node") {
		t.Error("pnpm/action-setup must come before actions/setup-node")
	}
}

func TestWorkflowYAMLMinimalProfile(t *testing.T) {
	pl := plan.Plan{
		Steps: []plan.Step{
			{ID: "x", Name: "Build", Category: plan.CatBuild, Commands: []string{"make"}},
		},
	}
	data, err := WorkflowYAML(pl)
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	if strings.Contains(y, "with:") || strings.Contains(y, "setup-") {
		t.Errorf("no setup steps expected for empty profile, got:\n%s", y)
	}
	if !strings.Contains(y, "      - name: Build\n        run: |\n          make\n") {
		t.Errorf("step not rendered as expected, got:\n%s", y)
	}
}

func TestWorkflowYAMLNoVersionHints(t *testing.T) {
	prof := profile.Profile{PackageManager: "npm"}
	prof.AddLanguage("node", "", 0.9)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	if !strings.Contains(y, "uses: actions/setup-node@v4") {
		t.Errorf("setup-node missing:\n%s", y)
	}
	if strings.Contains(y, "node-version:") {
		t.Errorf("empty version hint should omit node-version:\n%s", y)
	}
	if strings.Contains(y, "pnpm/action-setup") {
		t.Errorf("npm profile must not set up pnpm:\n%s", y)
	}
}
