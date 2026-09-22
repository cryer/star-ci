package render

import (
	"strings"
	"testing"

	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
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

func TestWorkflowYAMLRust(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("rust", "1.75.0", 0.98)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Set up Rust\n",
		"uses: dtolnay/rust-toolchain@stable",
		"          toolchain: '1.75.0'",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	var noHint profile.Profile
	noHint.AddLanguage("rust", "", 0.98)
	data, err = WorkflowYAML(plan.Plan{Profile: noHint})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y = string(data)
	if !strings.Contains(y, "uses: dtolnay/rust-toolchain@stable") {
		t.Errorf("rust setup missing:\n%s", y)
	}
	if strings.Contains(y, "toolchain:") {
		t.Errorf("empty version hint should omit toolchain:\n%s", y)
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

func TestWorkflowYAMLJava(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("java", "17", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Set up Java\n",
		"uses: actions/setup-java@v4",
		"          distribution: temurin",
		"          java-version: '17'",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	var noHint profile.Profile
	noHint.AddLanguage("java", "", 0.95)
	data, err = WorkflowYAML(plan.Plan{Profile: noHint})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y = string(data)
	if !strings.Contains(y, "distribution: temurin") {
		t.Errorf("java setup must always pin a distribution:\n%s", y)
	}
	if strings.Contains(y, "java-version:") {
		t.Errorf("empty version hint should omit java-version:\n%s", y)
	}
}

func TestWorkflowYAMLRuby(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("ruby", "3.2.2", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Set up Ruby\n",
		"uses: ruby/setup-ruby@v1",
		"          ruby-version: '3.2.2'",
		"          bundler-cache: true",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}
}

func TestWorkflowYAMLPHP(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("php", "8.1", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Set up PHP\n",
		"uses: shivammathur/setup-php@v2",
		"          php-version: '8.1'",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}
}

func TestWorkflowYAMLDotnet(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("dotnet", "8.0.100", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Set up .NET\n",
		"uses: actions/setup-dotnet@v4",
		"          dotnet-version: '8.0.100'",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}
}

func TestWorkflowYAMLCppNoSetup(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("cpp", "17", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	if strings.Contains(y, "setup-") {
		t.Errorf("cpp needs no setup step (runner ships cmake/gcc), got:\n%s", y)
	}
}

func TestWorkflowMatrixYAMLIdenticalPlans(t *testing.T) {
	mkPlan := func() plan.Plan {
		prof := profile.Profile{PackageManager: "pnpm"}
		prof.AddLanguage("node", "20", 0.9)
		return plan.Plan{
			Profile: prof,
			Steps: []plan.Step{
				{ID: "node-install", Name: "Install dependencies (pnpm)", Category: plan.CatInstall,
					Commands: []string{"pnpm install --frozen-lockfile"}},
				{ID: "node-test", Name: "Run tests (pnpm)", Category: plan.CatTest,
					Commands: []string{"pnpm test"}},
			},
		}
	}
	// unsorted input must still render a sorted matrix
	data, err := WorkflowMatrixYAML([]WorkspacePlan{
		{Path: "packages/web", Plan: mkPlan()},
		{Path: "packages/api", Plan: mkPlan()},
	})
	if err != nil {
		t.Fatalf("WorkflowMatrixYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"  ci:\n",
		"    strategy:\n      fail-fast: false\n      matrix:\n        workspace:\n          - packages/api\n          - packages/web\n",
		"        working-directory: ${{ matrix.workspace }}",
		"uses: actions/setup-node@v4",
		"      - name: Install dependencies (pnpm)",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("matrix workflow missing %q\ngot:\n%s", want, y)
		}
	}
	if strings.Contains(y, "defaults:") {
		t.Errorf("identical plans must not fall back to per-workspace jobs:\n%s", y)
	}
	// every run step carries the matrix working-directory
	if got := strings.Count(y, "working-directory: ${{ matrix.workspace }}"); got != 2 {
		t.Errorf("working-directory occurrences = %d, want 2 (one per run step)", got)
	}
}

func TestWorkflowMatrixYAMLDifferentPlans(t *testing.T) {
	webProf := profile.Profile{PackageManager: "pnpm"}
	webProf.AddLanguage("node", "20", 0.9)
	web := plan.Plan{
		Profile: webProf,
		Steps: []plan.Step{
			{ID: "node-install", Name: "Install dependencies (pnpm)", Category: plan.CatInstall,
				Commands: []string{"pnpm install --frozen-lockfile"}},
		},
	}
	var goProf profile.Profile
	goProf.AddLanguage("go", "1.22", 0.98)
	api := plan.Plan{
		Profile: goProf,
		Steps: []plan.Step{
			{ID: "go-test", Name: "Run tests (go test)", Category: plan.CatTest,
				Commands: []string{"go test ./..."}},
		},
	}

	data, err := WorkflowMatrixYAML([]WorkspacePlan{
		{Path: "packages/api", Plan: api},
		{Path: "packages/web", Plan: web},
	})
	if err != nil {
		t.Fatalf("WorkflowMatrixYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"  ci-packages-api:\n    name: ci (packages/api)\n",
		"  ci-packages-web:\n    name: ci (packages/web)\n",
		"    defaults:\n      run:\n        working-directory: packages/api\n",
		"        working-directory: packages/web\n",
		"uses: actions/setup-go@v5",
		"uses: actions/setup-node@v4",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("fallback workflow missing %q\ngot:\n%s", want, y)
		}
	}
	if strings.Contains(y, "matrix:") {
		t.Errorf("different plans must not render a matrix:\n%s", y)
	}
}

func TestWorkflowMatrixYAMLEmpty(t *testing.T) {
	if _, err := WorkflowMatrixYAML(nil); err == nil {
		t.Error("expected error for no workspace plans")
	}
}

func TestWorkflowYAMLRustCache(t *testing.T) {
	prof := profile.Profile{Lockfiles: []string{"Cargo.lock"}}
	prof.AddLanguage("rust", "1.75.0", 0.98)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Cache Cargo\n",
		"uses: actions/cache@v4",
		"          path: |\n            ~/.cargo/registry\n            ~/.cargo/git\n            target\n",
		"          key: ${{ runner.os }}-cargo-${{ hashFiles('Cargo.lock') }}\n",
		"          restore-keys: |\n            ${{ runner.os }}-cargo-\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	// no Cargo.lock -> no cache step
	var noLock profile.Profile
	noLock.AddLanguage("rust", "1.75.0", 0.98)
	data, err = WorkflowYAML(plan.Plan{Profile: noLock})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	if strings.Contains(string(data), "actions/cache") {
		t.Errorf("cargo cache must not render without Cargo.lock:\n%s", data)
	}
}

func TestWorkflowYAMLPHPCache(t *testing.T) {
	prof := profile.Profile{Lockfiles: []string{"composer.lock"}}
	prof.AddLanguage("php", "8.1", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Cache Composer\n",
		"uses: actions/cache@v4",
		"          path: |\n            ~/.cache/composer\n",
		"          key: ${{ runner.os }}-composer-${{ hashFiles('composer.lock') }}\n",
		"          restore-keys: |\n            ${{ runner.os }}-composer-\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	// no composer.lock -> no cache step (TestWorkflowYAMLPHP covers setup
	// only; assert absence here with an explicit no-lock profile)
	var noLock profile.Profile
	noLock.AddLanguage("php", "8.1", 0.95)
	data, err = WorkflowYAML(plan.Plan{Profile: noLock})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	if strings.Contains(string(data), "actions/cache") {
		t.Errorf("composer cache must not render without composer.lock:\n%s", data)
	}
}

func TestWorkflowYAMLDotnetCache(t *testing.T) {
	// no packages.lock.json -> key falls back to the csproj glob
	var prof profile.Profile
	prof.AddLanguage("dotnet", "8.0.100", 0.95)
	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"      - name: Cache NuGet\n",
		"          path: |\n            ~/.nuget/packages\n",
		"          key: ${{ runner.os }}-nuget-${{ hashFiles('**/*.csproj') }}\n",
		"          restore-keys: |\n            ${{ runner.os }}-nuget-\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	// packages.lock.json wins when present
	locked := profile.Profile{Lockfiles: []string{"packages.lock.json"}}
	locked.AddLanguage("dotnet", "8.0.100", 0.95)
	data, err = WorkflowYAML(plan.Plan{Profile: locked})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y = string(data)
	if !strings.Contains(y, "hashFiles('packages.lock.json')") {
		t.Errorf("nuget cache key must prefer packages.lock.json:\n%s", y)
	}
	if strings.Contains(y, "hashFiles('**/*.csproj')") {
		t.Errorf("csproj glob must not be used when packages.lock.json exists:\n%s", y)
	}
}

func TestWorkflowYAMLPythonCache(t *testing.T) {
	// pip: requirements manifest present -> cache: pip with a glob dependency path
	pip := profile.Profile{Lockfiles: []string{"requirements.txt"}}
	pip.AddLanguage("python", "3.11", 0.95)
	data, err := WorkflowYAML(plan.Plan{Profile: pip})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"          cache: pip\n",
		"          cache-dependency-path: requirements*.txt\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("workflow missing %q\ngot:\n%s", want, y)
		}
	}

	// poetry.lock -> cache: poetry, root default dependency path omitted
	poetry := profile.Profile{Lockfiles: []string{"poetry.lock"}}
	poetry.AddLanguage("python", "3.11", 0.95)
	data, err = WorkflowYAML(plan.Plan{Profile: poetry})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y = string(data)
	if !strings.Contains(y, "          cache: poetry\n") {
		t.Errorf("poetry cache missing:\n%s", y)
	}
	if strings.Contains(y, "cache-dependency-path") {
		t.Errorf("root poetry cache should rely on the default dependency path:\n%s", y)
	}

	// no dependency manifest -> no cache key (setup-python would fail)
	var bare profile.Profile
	bare.AddLanguage("python", "3.11", 0.95)
	data, err = WorkflowYAML(plan.Plan{Profile: bare})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y = string(data)
	if strings.Contains(y, "cache:") {
		t.Errorf("python cache must not render without a dependency manifest:\n%s", y)
	}
}

func TestWorkflowYAMLJavaCache(t *testing.T) {
	var prof profile.Profile
	prof.AddLanguage("java", "17", 0.95)
	prof.AddSignal("pom.xml", "build_tool", "maven", 0.95)

	data, err := WorkflowYAML(plan.Plan{Profile: prof})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	y := string(data)
	if !strings.Contains(y, "          cache: maven\n") {
		t.Errorf("setup-java cache missing:\n%s", y)
	}

	// no build tool signal -> no cache key
	var bare profile.Profile
	bare.AddLanguage("java", "17", 0.95)
	data, err = WorkflowYAML(plan.Plan{Profile: bare})
	if err != nil {
		t.Fatalf("WorkflowYAML: %v", err)
	}
	if strings.Contains(string(data), "cache:") {
		t.Errorf("java cache must not render without a detected build tool:\n%s", data)
	}
}

func TestWorkflowMatrixYAMLCachePaths(t *testing.T) {
	mkRust := func() plan.Plan {
		prof := profile.Profile{Lockfiles: []string{"Cargo.lock"}}
		prof.AddLanguage("rust", "1.75.0", 0.98)
		return plan.Plan{
			Profile: prof,
			Steps: []plan.Step{
				{ID: "rust-test", Name: "Run tests (cargo test)", Category: plan.CatTest,
					Commands: []string{"cargo test --locked"}},
			},
		}
	}
	data, err := WorkflowMatrixYAML([]WorkspacePlan{
		{Path: "crates/a", Plan: mkRust()},
		{Path: "crates/b", Plan: mkRust()},
	})
	if err != nil {
		t.Fatalf("WorkflowMatrixYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"          key: ${{ runner.os }}-cargo-${{ hashFiles(format('{0}/Cargo.lock', matrix.workspace)) }}\n",
		"            ${{ matrix.workspace }}/target\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("matrix workflow missing %q\ngot:\n%s", want, y)
		}
	}
}

func TestWorkflowMatrixYAMLNodeCacheDependencyPath(t *testing.T) {
	mkNode := func() plan.Plan {
		prof := profile.Profile{
			PackageManager: "pnpm",
			Lockfiles:      []string{"pnpm-lock.yaml"},
		}
		prof.AddLanguage("node", "20", 0.9)
		return plan.Plan{
			Profile: prof,
			Steps: []plan.Step{
				{ID: "node-install", Name: "Install dependencies (pnpm)", Category: plan.CatInstall,
					Commands: []string{"pnpm install --frozen-lockfile"}},
			},
		}
	}
	data, err := WorkflowMatrixYAML([]WorkspacePlan{
		{Path: "packages/a", Plan: mkNode()},
		{Path: "packages/b", Plan: mkNode()},
	})
	if err != nil {
		t.Fatalf("WorkflowMatrixYAML: %v", err)
	}
	y := string(data)
	if !strings.Contains(y, "          cache-dependency-path: ${{ matrix.workspace }}/pnpm-lock.yaml\n") {
		t.Errorf("setup-node cache-dependency-path missing workspace prefix:\n%s", y)
	}
}

func TestWorkflowMatrixYAMLFallbackCachePaths(t *testing.T) {
	rustProf := profile.Profile{Lockfiles: []string{"Cargo.lock"}}
	rustProf.AddLanguage("rust", "1.75.0", 0.98)
	rust := plan.Plan{
		Profile: rustProf,
		Steps: []plan.Step{
			{ID: "rust-test", Name: "Run tests (cargo test)", Category: plan.CatTest,
				Commands: []string{"cargo test --locked"}},
		},
	}
	goProf := profile.Profile{Lockfiles: []string{"go.sum"}}
	goProf.AddLanguage("go", "1.22", 0.98)
	goplan := plan.Plan{
		Profile: goProf,
		Steps: []plan.Step{
			{ID: "go-test", Name: "Run tests (go test)", Category: plan.CatTest,
				Commands: []string{"go test ./..."}},
		},
	}

	data, err := WorkflowMatrixYAML([]WorkspacePlan{
		{Path: "crates/lib", Plan: rust},
		{Path: "services/api", Plan: goplan},
	})
	if err != nil {
		t.Fatalf("WorkflowMatrixYAML: %v", err)
	}
	y := string(data)
	for _, want := range []string{
		"          key: ${{ runner.os }}-cargo-${{ hashFiles('crates/lib/Cargo.lock') }}\n",
		"            crates/lib/target\n",
		"          cache-dependency-path: services/api/go.sum\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("fallback workflow missing %q\ngot:\n%s", want, y)
		}
	}
}
