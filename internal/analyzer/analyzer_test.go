package analyzer_test

import (
	"path/filepath"
	"testing"

	"github.com/star-ci/star-ci/internal/analyzer"
	"github.com/star-ci/star-ci/internal/profile"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func hasString(slice []string, v string) bool {
	for _, s := range slice {
		if s == v {
			return true
		}
	}
	return false
}

func hasSignal(prof profile.Profile, key, value string) bool {
	for _, s := range prof.Signals {
		if s.Key == key && s.Value == value {
			return true
		}
	}
	return false
}

func TestNodePnpm(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "node-pnpm"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("node") {
		t.Fatalf("node not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("node"); got != "20" {
		t.Errorf("version hint = %q, want 20", got)
	}
	if prof.Languages[0].Name != "node" {
		t.Errorf("primary language = %q, want node", prof.Languages[0].Name)
	}
	if prof.PackageManager != "pnpm" {
		t.Errorf("PackageManager = %q, want pnpm", prof.PackageManager)
	}
	if !hasString(prof.Lockfiles, "pnpm-lock.yaml") {
		t.Errorf("Lockfiles = %v, want pnpm-lock.yaml", prof.Lockfiles)
	}
	if prof.TestRunner != "vitest" {
		t.Errorf("TestRunner = %q, want vitest", prof.TestRunner)
	}
	if !hasString(prof.Linters, "eslint") {
		t.Errorf("Linters = %v, want eslint", prof.Linters)
	}
	if prof.Typecheck != "tsc" {
		t.Errorf("Typecheck = %q, want tsc", prof.Typecheck)
	}
	if prof.Scripts["build"] != "next build" {
		t.Errorf("Scripts[build] = %q, want %q", prof.Scripts["build"], "next build")
	}
	if !hasSignal(prof, "framework", "next") {
		t.Error("missing framework=next signal")
	}
	if !hasSignal(prof, "package_manager", "pnpm") {
		t.Error("missing package_manager=pnpm signal")
	}
}

func TestPythonPytest(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "python-pytest"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("python") {
		t.Fatalf("python not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("python"); got != "3.11" {
		t.Errorf("version hint = %q, want 3.11", got)
	}
	if prof.PackageManager != "uv" {
		t.Errorf("PackageManager = %q, want uv", prof.PackageManager)
	}
	if !hasString(prof.Lockfiles, "uv.lock") {
		t.Errorf("Lockfiles = %v, want uv.lock", prof.Lockfiles)
	}
	if prof.TestRunner != "pytest" {
		t.Errorf("TestRunner = %q, want pytest", prof.TestRunner)
	}
	if !hasString(prof.Linters, "ruff") {
		t.Errorf("Linters = %v, want ruff", prof.Linters)
	}
}

func TestGoMod(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "go-mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("go") {
		t.Fatalf("go not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("go"); got != "1.22" {
		t.Errorf("version hint = %q, want 1.22", got)
	}
	for _, l := range prof.Languages {
		if l.Name == "go" && l.Confidence != 0.98 {
			t.Errorf("go confidence = %v, want 0.98", l.Confidence)
		}
	}
	if prof.PackageManager != "go" {
		t.Errorf("PackageManager = %q, want go", prof.PackageManager)
	}
	if prof.TestRunner != "go" {
		t.Errorf("TestRunner = %q, want go", prof.TestRunner)
	}
	if prof.Typecheck != "govet" {
		t.Errorf("Typecheck = %q, want govet", prof.Typecheck)
	}
	if !hasString(prof.Linters, "golangci-lint") {
		t.Errorf("Linters = %v, want golangci-lint", prof.Linters)
	}
	if !hasString(prof.Lockfiles, "go.sum") {
		t.Errorf("Lockfiles = %v, want go.sum", prof.Lockfiles)
	}
	if !prof.HasDockerfile {
		t.Error("HasDockerfile = false, want true")
	}
	if !hasString(prof.ExistingCI, "github-actions") {
		t.Errorf("ExistingCI = %v, want github-actions", prof.ExistingCI)
	}
}

func TestGoVendorTestsIgnored(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "go-no-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("go") {
		t.Fatalf("go not detected: %+v", prof.Languages)
	}
	if prof.TestRunner != "" {
		t.Errorf("TestRunner = %q, want empty (test file lives in vendor/)", prof.TestRunner)
	}
}

func TestEmptyDir(t *testing.T) {
	prof, err := analyzer.Analyze(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(prof.Languages) != 0 {
		t.Errorf("Languages = %v, want none", prof.Languages)
	}
	if len(prof.Signals) != 0 {
		t.Errorf("Signals = %v, want none", prof.Signals)
	}
}

func TestMissingRoot(t *testing.T) {
	if _, err := analyzer.Analyze(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error for missing root")
	}
}
