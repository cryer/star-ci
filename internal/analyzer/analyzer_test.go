package analyzer_test

import (
	"path/filepath"
	"testing"

	"github.com/cryer/star-ci/internal/analyzer"
	"github.com/cryer/star-ci/internal/profile"
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
	if !hasString(prof.Lockfiles, "requirements.txt") {
		t.Errorf("Lockfiles = %v, want requirements.txt", prof.Lockfiles)
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

func TestNodeTurboMonorepo(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "node-turbo"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("node") {
		t.Fatalf("node not detected: %+v", prof.Languages)
	}
	if !hasSignal(prof, "monorepo", "turbo") {
		t.Error("missing monorepo=turbo signal")
	}
	if !hasSignal(prof, "framework", "vite") {
		t.Error("missing framework=vite signal")
	}
	if hasSignal(prof, "monorepo", "nx") {
		t.Error("unexpected monorepo=nx signal")
	}
}

func TestRustCargo(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "rust-cargo"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("rust") {
		t.Fatalf("rust not detected: %+v", prof.Languages)
	}
	// rust-toolchain.toml channel wins over Cargo.toml rust-version
	if got := prof.LanguageVersion("rust"); got != "1.75.0" {
		t.Errorf("version hint = %q, want 1.75.0", got)
	}
	if prof.PackageManager != "cargo" {
		t.Errorf("PackageManager = %q, want cargo", prof.PackageManager)
	}
	if !hasString(prof.Lockfiles, "Cargo.lock") {
		t.Errorf("Lockfiles = %v, want Cargo.lock", prof.Lockfiles)
	}
	if prof.TestRunner != "cargo" {
		t.Errorf("TestRunner = %q, want cargo", prof.TestRunner)
	}
	if !hasString(prof.Formatters, "rustfmt") {
		t.Errorf("Formatters = %v, want rustfmt", prof.Formatters)
	}
	if hasString(prof.Linters, "clippy") {
		t.Errorf("Linters = %v, want no clippy without clippy.toml", prof.Linters)
	}
}

func TestRustToolchainPlain(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "rust-toolchain-plain"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("rust") {
		t.Fatalf("rust not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("rust"); got != "stable" {
		t.Errorf("version hint = %q, want stable", got)
	}
	if hasString(prof.Lockfiles, "Cargo.lock") {
		t.Errorf("Lockfiles = %v, want no Cargo.lock", prof.Lockfiles)
	}
	if len(prof.Formatters) != 0 {
		t.Errorf("Formatters = %v, want none without rustfmt.toml", prof.Formatters)
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

func TestJavaMaven(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "java-maven"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("java") {
		t.Fatalf("java not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("java"); got != "17" {
		t.Errorf("version hint = %q, want 17", got)
	}
	if prof.PackageManager != "maven" {
		t.Errorf("PackageManager = %q, want maven", prof.PackageManager)
	}
	if !hasSignal(prof, "build_tool", "maven") {
		t.Error("missing build_tool=maven signal")
	}
	if !hasSignal(prof, "wrapper", "mvnw") {
		t.Error("missing wrapper=mvnw signal")
	}
}

func TestJavaGradle(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "java-gradle"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("java") {
		t.Fatalf("java not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("java"); got != "21" {
		t.Errorf("version hint = %q, want 21", got)
	}
	if prof.PackageManager != "gradle" {
		t.Errorf("PackageManager = %q, want gradle", prof.PackageManager)
	}
	if !hasSignal(prof, "build_tool", "gradle") {
		t.Error("missing build_tool=gradle signal")
	}
	if !hasSignal(prof, "wrapper", "gradlew") {
		t.Error("missing wrapper=gradlew signal")
	}
}

func TestRubyRspec(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "ruby-rspec"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("ruby") {
		t.Fatalf("ruby not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("ruby"); got != "3.2.2" {
		t.Errorf("version hint = %q, want 3.2.2", got)
	}
	if prof.PackageManager != "bundler" {
		t.Errorf("PackageManager = %q, want bundler", prof.PackageManager)
	}
	if !hasString(prof.Lockfiles, "Gemfile.lock") {
		t.Errorf("Lockfiles = %v, want Gemfile.lock", prof.Lockfiles)
	}
	if prof.TestRunner != "rspec" {
		t.Errorf("TestRunner = %q, want rspec", prof.TestRunner)
	}
	if !hasString(prof.Linters, "rubocop") {
		t.Errorf("Linters = %v, want rubocop", prof.Linters)
	}
}

func TestRubyRake(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "ruby-rake"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("ruby") {
		t.Fatalf("ruby not detected: %+v", prof.Languages)
	}
	// .ruby-version wins when the Gemfile has no ruby directive
	if got := prof.LanguageVersion("ruby"); got != "3.3.0" {
		t.Errorf("version hint = %q, want 3.3.0", got)
	}
	if prof.TestRunner != "rake" {
		t.Errorf("TestRunner = %q, want rake", prof.TestRunner)
	}
	if hasString(prof.Linters, "rubocop") {
		t.Errorf("Linters = %v, want no rubocop without .rubocop.yml", prof.Linters)
	}
}

func TestPHPComposer(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "php-composer"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("php") {
		t.Fatalf("php not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("php"); got != "8.1" {
		t.Errorf("version hint = %q, want 8.1", got)
	}
	if prof.PackageManager != "composer" {
		t.Errorf("PackageManager = %q, want composer", prof.PackageManager)
	}
	if !hasString(prof.Lockfiles, "composer.lock") {
		t.Errorf("Lockfiles = %v, want composer.lock", prof.Lockfiles)
	}
	if prof.TestRunner != "phpunit" {
		t.Errorf("TestRunner = %q, want phpunit (require-dev)", prof.TestRunner)
	}
}

func TestPHPPhpunitXML(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "php-phpunit-xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("php") {
		t.Fatalf("php not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("php"); got != "8.2" {
		t.Errorf("version hint = %q, want 8.2", got)
	}
	if !hasSignal(prof, "test_runner", "phpunit") {
		t.Error("missing test_runner=phpunit signal from phpunit.xml.dist")
	}
	if hasString(prof.Lockfiles, "composer.lock") {
		t.Errorf("Lockfiles = %v, want no composer.lock", prof.Lockfiles)
	}
}

func TestDotnetSln(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "dotnet-sln"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("dotnet") {
		t.Fatalf("dotnet not detected: %+v", prof.Languages)
	}
	// global.json sdk version wins over TargetFramework
	if got := prof.LanguageVersion("dotnet"); got != "8.0.100" {
		t.Errorf("version hint = %q, want 8.0.100", got)
	}
	if prof.PackageManager != "nuget" {
		t.Errorf("PackageManager = %q, want nuget", prof.PackageManager)
	}
	if !hasString(prof.Lockfiles, "packages.lock.json") {
		t.Errorf("Lockfiles = %v, want packages.lock.json", prof.Lockfiles)
	}
	if !hasSignal(prof, "test_runner", "dotnet") {
		t.Error("missing test_runner=dotnet signal (test project present)")
	}
}

func TestDotnetNoTests(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "dotnet-no-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("dotnet") {
		t.Fatalf("dotnet not detected: %+v", prof.Languages)
	}
	if got := prof.LanguageVersion("dotnet"); got != "net7.0" {
		t.Errorf("version hint = %q, want net7.0 (TargetFramework)", got)
	}
	if hasSignal(prof, "test_runner", "dotnet") {
		t.Error("unexpected test_runner=dotnet signal without a test project")
	}
}

func TestCppCMake(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "cpp-cmake"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("cpp") {
		t.Fatalf("cpp not detected: %+v", prof.Languages)
	}
	// CMAKE_CXX_STANDARD_REQUIRED must not shadow CMAKE_CXX_STANDARD
	if got := prof.LanguageVersion("cpp"); got != "17" {
		t.Errorf("version hint = %q, want 17", got)
	}
	if !hasSignal(prof, "build_tool", "cmake") {
		t.Error("missing build_tool=cmake signal")
	}
	if !hasSignal(prof, "test_runner", "ctest") {
		t.Error("missing test_runner=ctest signal (enable_testing present)")
	}
}

func TestCppMake(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "cpp-make"))
	if err != nil {
		t.Fatal(err)
	}
	if !prof.HasLanguage("cpp") {
		t.Fatalf("cpp not detected: %+v", prof.Languages)
	}
	for _, l := range prof.Languages {
		if l.Name == "cpp" && l.Confidence != 0.7 {
			t.Errorf("cpp confidence = %v, want 0.7 (no manifest)", l.Confidence)
		}
	}
	if !hasSignal(prof, "build_tool", "make") {
		t.Error("missing build_tool=make signal")
	}
	if !hasSignal(prof, "test_runner", "make test") {
		t.Error("missing test_runner=make test signal (test target present)")
	}
}

func TestCppMakeNoSources(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "cpp-make-no-src"))
	if err != nil {
		t.Fatal(err)
	}
	if prof.HasLanguage("cpp") {
		t.Errorf("cpp detected without sources: %+v", prof.Languages)
	}
}
