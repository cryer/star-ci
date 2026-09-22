package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryer/star-ci/internal/plan"
)

func load(t *testing.T, content string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Load(dir)
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := load(t, "")
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if cfg.Loaded() {
		t.Error("Loaded() = true for missing file")
	}
	if cfg.Confidence != nil || len(cfg.Disable) != 0 || len(cfg.Append) != 0 {
		t.Errorf("expected zero config, got %+v", cfg)
	}
}

func TestLoadFull(t *testing.T) {
	cfg, err := load(t, `confidence: 0.7
disable:
  - node-security
  - go-lint
append:
  - id: docs-link-check
    name: Docs link check
    category: test
    commands:
      - npx markdown-link-check README.md
      - npx markdown-link-check docs/
    reason: declared in .star-ci.yml
    optional: true
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Loaded() {
		t.Error("Loaded() = false for existing file")
	}
	if cfg.Confidence == nil || *cfg.Confidence != 0.7 {
		t.Errorf("confidence = %v, want 0.7", cfg.Confidence)
	}
	if got := strings.Join(cfg.Disable, ","); got != "node-security,go-lint" {
		t.Errorf("disable = %q", got)
	}
	if len(cfg.Append) != 1 {
		t.Fatalf("append len = %d, want 1", len(cfg.Append))
	}
	s := cfg.Append[0]
	if s.ID != "docs-link-check" || s.Name != "Docs link check" || s.Category != plan.CatTest {
		t.Errorf("step = %+v", s)
	}
	if len(s.Commands) != 2 || s.Commands[0] != "npx markdown-link-check README.md" {
		t.Errorf("commands = %v", s.Commands)
	}
	if s.Reason != "declared in .star-ci.yml" || !s.Optional {
		t.Errorf("reason/optional = %q/%v", s.Reason, s.Optional)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(t, `append:
  - id: extra
    commands:
      - make extra
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s := cfg.Append[0]
	if s.Category != plan.CatTest {
		t.Errorf("default category = %q, want test", s.Category)
	}
	if s.Name != "extra" {
		t.Errorf("default name = %q, want id", s.Name)
	}
	if s.Reason != "declared in .star-ci.yml" {
		t.Errorf("default reason = %q", s.Reason)
	}
	if s.Optional {
		t.Error("default optional = true, want false")
	}
}

func TestLoadMultipleAppendItems(t *testing.T) {
	cfg, err := load(t, `append:
  - id: a
    category: build
    commands:
      - make a
  - id: b
    commands:
      - make b
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Append) != 2 || cfg.Append[0].ID != "a" || cfg.Append[1].ID != "b" {
		t.Fatalf("append = %+v", cfg.Append)
	}
	if cfg.Append[0].Category != plan.CatBuild {
		t.Errorf("category = %q, want build", cfg.Append[0].Category)
	}
}

func TestLoadQuotedValues(t *testing.T) {
	cfg, err := load(t, `disable:
  - "go-lint"
append:
  - id: 'quoted-id'
    commands:
      - "echo hi"
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Disable[0] != "go-lint" || cfg.Append[0].ID != "quoted-id" || cfg.Append[0].Commands[0] != "echo hi" {
		t.Errorf("quotes not stripped: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"unknown top-level key", "bogus: 1\n", ":1: unknown top-level key"},
		{"confidence not a number", "confidence: high\n", ":1: invalid confidence"},
		{"confidence out of range", "confidence: 1.5\n", ":1: confidence 1.50 out of range"},
		{"confidence missing value", "confidence:\n", ":1: confidence expects a numeric value"},
		{"disable not a list", "disable:\n  go-lint\n", ":2: disable expects list items"},
		{"indented without section", "  - go-lint\n", ":1: unexpected indented line"},
		{"append item missing id", "append:\n  - name: nope\n    commands:\n      - make x\n", ":2: append item missing required 'id'"},
		{"append item no commands", "append:\n  - id: x\n", "has no commands"},
		{"unknown category", "append:\n  - id: x\n    category: wat\n    commands:\n      - make x\n", ":3: unknown category"},
		{"bad optional", "append:\n  - id: x\n    optional: yes\n    commands:\n      - make x\n", ":3: optional must be true or false"},
		{"unknown step field", "append:\n  - id: x\n    bogus: 1\n    commands:\n      - make x\n", ":3: unknown step field"},
		{"tab indentation", "disable:\n\t- go-lint\n", ":2: tabs are not allowed"},
		{"commands inline value", "append:\n  - id: x\n    commands: make x\n", ":3: commands expects a list"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, tc.content)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadCoverage(t *testing.T) {
	cfg, err := load(t, "coverage: 80\nconfidence: 0.7\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Coverage == nil || *cfg.Coverage != 80 {
		t.Errorf("coverage = %v, want 80", cfg.Coverage)
	}
	if cfg.Confidence == nil || *cfg.Confidence != 0.7 {
		t.Errorf("confidence = %v, want 0.7", cfg.Confidence)
	}

	cfg, err = load(t, "coverage: 0\ndisable:\n  - go-lint\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Coverage == nil || *cfg.Coverage != 0 {
		t.Errorf("coverage = %v, want 0 (boundary)", cfg.Coverage)
	}
	if len(cfg.Disable) != 1 {
		t.Errorf("coverage key must not swallow the following section: %+v", cfg)
	}
}

func TestLoadCoverageErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"coverage not a number", "coverage: high\n", ":1: invalid coverage"},
		{"coverage above 100", "coverage: 101\n", ":1: coverage 101.00 out of range [0, 100]"},
		{"coverage below 0", "coverage: -1\n", ":1: coverage -1.00 out of range [0, 100]"},
		{"coverage missing value", "coverage:\n", ":1: coverage expects a numeric value"},
		{"coverage wrong line reported", "disable:\n  - x\ncoverage: 150\n", ":3: coverage 150.00 out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, tc.content)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	cfg := &Config{
		Disable: []string{"go-lint"},
		Append: []plan.Step{{
			ID:       "zz-docs",
			Category: plan.CatLint,
			Commands: []string{"make docs"},
		}},
	}
	p := plan.Plan{Steps: []plan.Step{
		{ID: "go-lint", Category: plan.CatLint},
		{ID: "go-test", Category: plan.CatTest},
		{ID: "go-build", Category: plan.CatBuild},
	}}
	p.Sort()
	cfg.Apply(&p)

	var ids []string
	for _, s := range p.Steps {
		ids = append(ids, s.ID)
	}
	got := strings.Join(ids, ",")
	if got != "zz-docs,go-test,go-build" {
		t.Errorf("steps = %q, want disabled dropped, appended sorted in", got)
	}
}

func TestApplyEmptyConfigIsNoop(t *testing.T) {
	p := plan.Plan{Steps: []plan.Step{{ID: "go-test", Category: plan.CatTest}}}
	p.Sort()
	cfg := &Config{}
	cfg.Apply(&p)
	if len(p.Steps) != 1 || p.Steps[0].ID != "go-test" {
		t.Errorf("empty config mutated plan: %+v", p.Steps)
	}
}
