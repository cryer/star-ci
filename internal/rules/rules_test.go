package rules

import (
	"testing"

	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
)

func findStep(t *testing.T, pl plan.Plan, id string) plan.Step {
	t.Helper()
	for _, s := range pl.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("step %s not found in plan (have %v)", id, stepIDs(pl))
	return plan.Step{}
}

func stepIDs(pl plan.Plan) []string {
	var ids []string
	for _, s := range pl.Steps {
		ids = append(ids, s.ID)
	}
	return ids
}

func assertStep(t *testing.T, pl plan.Plan, id string, cmds []string, cat plan.Category, optional bool) {
	t.Helper()
	s := findStep(t, pl, id)
	if s.Category != cat {
		t.Errorf("step %s: category = %q, want %q", id, s.Category, cat)
	}
	if s.Optional != optional {
		t.Errorf("step %s: optional = %v, want %v", id, s.Optional, optional)
	}
	if len(s.Commands) != len(cmds) {
		t.Fatalf("step %s: commands = %v, want %v", id, s.Commands, cmds)
	}
	for i, c := range cmds {
		if s.Commands[i] != c {
			t.Errorf("step %s: command[%d] = %q, want %q", id, i, s.Commands[i], c)
		}
	}
	if s.Reason == "" {
		t.Errorf("step %s: empty reason", id)
	}
}

func assertNoStep(t *testing.T, pl plan.Plan, id string) {
	t.Helper()
	for _, s := range pl.Steps {
		if s.ID == id {
			t.Errorf("unexpected step %s in plan", id)
		}
	}
}

func TestBuildPlanNode(t *testing.T) {
	tests := []struct {
		name string
		prof profile.Profile
		want map[string][]string // step ID -> commands
		omit []string
	}{
		{
			name: "pnpm full setup",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "node", VersionHint: "20", Confidence: 0.95}},
				PackageManager: "pnpm",
				TestRunner:     "vitest",
				Linters:        []string{"eslint"},
				Formatters:     []string{"prettier"},
				Typecheck:      "tsc",
				Lockfiles:      []string{"pnpm-lock.yaml"},
				Scripts:        map[string]string{"lint": "eslint .", "build": "vite build", "test": "vitest"},
				Signals: []profile.Signal{
					{Source: "package.json", Key: "package_manager", Value: "pnpm", Confidence: 0.95},
					{Source: ".prettierrc", Key: "formatter", Value: "prettier", Confidence: 0.9},
				},
			},
			want: map[string][]string{
				"node-install":        {"pnpm install --frozen-lockfile"},
				"node-lint":           {"pnpm run lint"},
				"node-format":         {"npx prettier --check ."},
				"node-typecheck":      {"npx tsc --noEmit"},
				"node-test":           {"pnpm test"},
				"node-build":          {"pnpm run build"},
				"security-audit-node": {"npm audit --audit-level=high"},
				"security-secrets":    {"gitleaks detect --source ."},
			},
		},
		{
			name: "npm minimal, eslint without lint script",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "node", Confidence: 0.9}},
				PackageManager: "npm",
				Linters:        []string{"eslint", "biome"},
			},
			want: map[string][]string{
				"node-install": {"npm ci"},
				"node-lint":    {"npx eslint .", "npx biome check ."},
			},
			omit: []string{"node-format", "node-typecheck", "node-test", "node-build"},
		},
		{
			name: "yarn and bun installs",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "node", Confidence: 0.9}},
				PackageManager: "yarn",
				Lockfiles:      []string{"yarn.lock"},
			},
			want: map[string][]string{"node-install": {"yarn install --frozen-lockfile"}},
		},
		{
			name: "bun install",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "node", Confidence: 0.9}},
				PackageManager: "bun",
			},
			want: map[string][]string{"node-install": {"bun install --frozen-lockfile"}},
		},
		{
			name: "typecheck script override",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "node", Confidence: 0.9}},
				Typecheck:  "tsc",
				Scripts:    map[string]string{"typecheck": "tsc --noEmit"},
				TestRunner: "jest",
			},
			want: map[string][]string{
				"node-typecheck": {"npm run typecheck"},
				"node-test":      {"npm test"},
			},
		},
		{
			name: "prettier without config signal or format script is skipped",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "node", Confidence: 0.9}},
				Formatters: []string{"prettier"},
			},
			omit: []string{"node-format"},
		},
		{
			name: "prettier with format script",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "node", Confidence: 0.9}},
				Formatters: []string{"prettier"},
				Scripts:    map[string]string{"format": "prettier --write ."},
			},
			want: map[string][]string{"node-format": {"npx prettier --check ."}},
		},
		{
			name: "turbo monorepo runs build/test via turbo",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "node", Confidence: 0.95}},
				TestRunner: "vitest",
				Scripts:    map[string]string{"build": "turbo run build", "test": "turbo run test"},
				Signals: []profile.Signal{
					{Source: "turbo.json", Key: "monorepo", Value: "turbo", Confidence: 0.95},
				},
			},
			want: map[string][]string{
				"node-test":  {"turbo run test"},
				"node-build": {"turbo run build"},
			},
		},
		{
			name: "nx monorepo runs build/test via nx",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "node", Confidence: 0.95}},
				TestRunner: "jest",
				Scripts:    map[string]string{"build": "vite build", "test": "jest"},
				Signals: []profile.Signal{
					{Source: "nx.json", Key: "monorepo", Value: "nx", Confidence: 0.95},
				},
			},
			want: map[string][]string{
				"node-test":  {"nx run-many -t test"},
				"node-build": {"nx run-many -t build"},
			},
		},
		{
			name: "low-confidence monorepo signal is ignored",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "node", Confidence: 0.9}},
				Scripts:   map[string]string{"build": "vite build"},
				Signals: []profile.Signal{
					{Source: "turbo.json", Key: "monorepo", Value: "turbo", Confidence: 0.4},
				},
			},
			want: map[string][]string{"node-build": {"npm run build"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := BuildPlan(tt.prof)
			for id, cmds := range tt.want {
				cat := plan.CatLint
				optional := false
				switch id {
				case "node-install":
					cat = plan.CatInstall
				case "node-typecheck":
					cat = plan.CatTypecheck
				case "node-test":
					cat = plan.CatTest
				case "node-build":
					cat = plan.CatBuild
				case "security-audit-node", "security-secrets":
					cat = plan.CatSecurity
					optional = true
				}
				assertStep(t, pl, id, cmds, cat, optional)
			}
			for _, id := range tt.omit {
				assertNoStep(t, pl, id)
			}
		})
	}
}

func TestBuildPlanNodeReason(t *testing.T) {
	p := profile.Profile{
		Languages:      []profile.Language{{Name: "node", Confidence: 0.95}},
		PackageManager: "pnpm",
		Lockfiles:      []string{"pnpm-lock.yaml"},
	}
	pl := BuildPlan(p)
	s := findStep(t, pl, "node-install")
	want := "package.json + pnpm-lock.yaml detected"
	if s.Reason != want {
		t.Errorf("reason = %q, want %q", s.Reason, want)
	}
}

func TestBuildPlanNodeFrameworkReason(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{{Name: "node", Confidence: 0.95}},
		Scripts:   map[string]string{"build": "next build"},
		Signals: []profile.Signal{
			{Source: "package.json", Key: "framework", Value: "next", Confidence: 0.9},
			{Source: "package.json", Key: "framework", Value: "vite", Confidence: 0.4}, // below threshold
		},
	}
	pl := BuildPlan(p)
	s := findStep(t, pl, "node-build")
	want := "package.json detected; build script detected; framework next"
	if s.Reason != want {
		t.Errorf("reason = %q, want %q", s.Reason, want)
	}
}

func TestBuildPlanPython(t *testing.T) {
	tests := []struct {
		name string
		prof profile.Profile
		want map[string][]string
		omit []string
	}{
		{
			name: "uv with pyproject",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "python", VersionHint: "3.11", Confidence: 0.9}},
				PackageManager: "uv",
				TestRunner:     "pytest",
				Linters:        []string{"ruff"},
				Formatters:     []string{"black"},
				Typecheck:      "mypy",
				Signals: []profile.Signal{
					{Source: "pyproject.toml", Key: "manifest", Value: "uv", Confidence: 0.9},
				},
			},
			want: map[string][]string{
				"python-install":   {"uv sync"},
				"python-lint":      {"ruff check ."},
				"python-format":    {"black --check ."},
				"python-typecheck": {"mypy ."},
				"python-test":      {"python -m pytest"},
			},
		},
		{
			name: "uv without pyproject falls back to requirements",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "python", Confidence: 0.9}},
				PackageManager: "uv",
			},
			want: map[string][]string{"python-install": {"uv pip install -r requirements.txt"}},
		},
		{
			name: "poetry",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "python", Confidence: 0.9}},
				PackageManager: "poetry",
			},
			want: map[string][]string{"python-install": {"poetry install"}},
		},
		{
			name: "pip with requirements.txt",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "python", Confidence: 0.9}},
				PackageManager: "pip",
				Signals: []profile.Signal{
					{Source: "requirements.txt", Key: "manifest", Value: "pip", Confidence: 0.9},
				},
			},
			want: map[string][]string{"python-install": {"pip install -r requirements.txt"}},
		},
		{
			name: "pip without requirements.txt",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "python", Confidence: 0.9}},
				PackageManager: "pip",
			},
			want: map[string][]string{"python-install": {"pip install -e ."}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := BuildPlan(tt.prof)
			for id, cmds := range tt.want {
				assertStep(t, pl, id, cmds, categoryOf(id), false)
			}
			for _, id := range tt.omit {
				assertNoStep(t, pl, id)
			}
			assertStep(t, pl, "security-audit-python",
				[]string{"pip install pip-audit && pip-audit"}, plan.CatSecurity, true)
		})
	}
}

func categoryOf(id string) plan.Category {
	switch {
	case hasSuffix(id, "-install"):
		return plan.CatInstall
	case hasSuffix(id, "-lint"), hasSuffix(id, "-format"):
		return plan.CatLint
	case hasSuffix(id, "-typecheck"):
		return plan.CatTypecheck
	case hasSuffix(id, "-test"):
		return plan.CatTest
	case hasSuffix(id, "-build"):
		return plan.CatBuild
	}
	return plan.CatSecurity
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestBuildPlanGo(t *testing.T) {
	p := profile.Profile{
		Languages:  []profile.Language{{Name: "go", VersionHint: "1.22", Confidence: 0.99}},
		Typecheck:  "govet",
		Linters:    []string{"golangci-lint"},
		TestRunner: "go",
		Signals: []profile.Signal{
			{Source: "go.mod", Key: "manifest", Value: "go", Confidence: 0.99},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "go-install", []string{"go mod download"}, plan.CatInstall, false)
	assertStep(t, pl, "go-lint", []string{"golangci-lint run"}, plan.CatLint, false)
	assertStep(t, pl, "go-typecheck", []string{"go vet ./..."}, plan.CatTypecheck, false)
	assertStep(t, pl, "go-test", []string{"go test ./..."}, plan.CatTest, false)
	assertStep(t, pl, "go-build", []string{"go build ./..."}, plan.CatBuild, false)
	assertStep(t, pl, "security-audit-go",
		[]string{"go run golang.org/x/vuln/cmd/govulncheck@latest ./..."}, plan.CatSecurity, true)
	assertStep(t, pl, "security-secrets", []string{"gitleaks detect --source ."}, plan.CatSecurity, true)
}

func TestBuildPlanGoMinimal(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{{Name: "go", Confidence: 0.9}},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "go-install", []string{"go mod download"}, plan.CatInstall, false)
	assertStep(t, pl, "go-test", []string{"go test ./..."}, plan.CatTest, false)
	assertStep(t, pl, "go-build", []string{"go build ./..."}, plan.CatBuild, false)
	assertNoStep(t, pl, "go-lint")
	assertNoStep(t, pl, "go-typecheck")
}

func TestBuildPlanRust(t *testing.T) {
	p := profile.Profile{
		Languages:      []profile.Language{{Name: "rust", VersionHint: "1.75.0", Confidence: 0.98}},
		PackageManager: "cargo",
		TestRunner:     "cargo",
		Linters:        []string{"clippy"},
		Formatters:     []string{"rustfmt"},
		Lockfiles:      []string{"Cargo.lock"},
		Signals: []profile.Signal{
			{Source: "Cargo.toml", Key: "language", Value: "rust", Confidence: 0.98},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "rust-install", []string{"cargo fetch"}, plan.CatInstall, false)
	assertStep(t, pl, "rust-lint", []string{"cargo clippy --all-targets -- -D warnings"}, plan.CatLint, false)
	assertStep(t, pl, "rust-format", []string{"cargo fmt --check"}, plan.CatLint, false)
	assertStep(t, pl, "rust-test", []string{"cargo test"}, plan.CatTest, false)
	assertStep(t, pl, "rust-build", []string{"cargo build --locked"}, plan.CatBuild, false)
	assertStep(t, pl, "security-audit-rust",
		[]string{"cargo install cargo-audit --locked && cargo audit"}, plan.CatSecurity, true)
	assertStep(t, pl, "security-secrets", []string{"gitleaks detect --source ."}, plan.CatSecurity, true)
	s := findStep(t, pl, "rust-install")
	if want := "Cargo.toml + Cargo.lock detected"; s.Reason != want {
		t.Errorf("rust-install reason = %q, want %q", s.Reason, want)
	}
}

func TestBuildPlanRustMinimal(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{{Name: "rust", Confidence: 0.98}},
		Signals: []profile.Signal{
			{Source: "Cargo.toml", Key: "language", Value: "rust", Confidence: 0.98},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "rust-install", []string{"cargo fetch"}, plan.CatInstall, false)
	assertStep(t, pl, "rust-test", []string{"cargo test"}, plan.CatTest, false)
	assertStep(t, pl, "rust-build", []string{"cargo build"}, plan.CatBuild, false)
	assertNoStep(t, pl, "rust-lint")
	assertNoStep(t, pl, "rust-format")
}

func TestBuildPlanDocker(t *testing.T) {
	p := profile.Profile{
		Languages:     []profile.Language{{Name: "go", Confidence: 0.9}},
		HasDockerfile: true,
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "docker-build", []string{"docker build ."}, plan.CatBuild, true)
}

func TestBuildPlanMinConfidence(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{
			{Name: "node", Confidence: 0.3},
			{Name: "go", Confidence: 0.8},
		},
		PackageManager: "npm",
	}
	pl := BuildPlan(p)
	assertNoStep(t, pl, "node-install")
	assertNoStep(t, pl, "security-audit-node")
	assertStep(t, pl, "go-install", []string{"go mod download"}, plan.CatInstall, false)
}

func TestBuildPlanOrdering(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{
			{Name: "go", Confidence: 0.9},
			{Name: "node", Confidence: 0.95},
		},
		PackageManager: "npm",
		TestRunner:     "jest",
		Scripts:        map[string]string{"build": "tsc"},
	}
	pl := BuildPlan(p)
	var cats []plan.Category
	for _, s := range pl.Steps {
		cats = append(cats, s.Category)
	}
	rank := map[plan.Category]int{}
	for i, c := range plan.CategoryOrder {
		rank[c] = i
	}
	for i := 1; i < len(cats); i++ {
		if rank[cats[i-1]] > rank[cats[i]] {
			t.Fatalf("steps out of category order: %v", stepIDs(pl))
		}
	}
}

func TestBuildPlanExistingCIIgnored(t *testing.T) {
	p := profile.Profile{
		Languages:  []profile.Language{{Name: "go", Confidence: 0.9}},
		ExistingCI: []string{"github-actions"},
	}
	pl := BuildPlan(p)
	if len(pl.Steps) == 0 {
		t.Fatal("plan should still be built when github-actions CI exists")
	}
	assertStep(t, pl, "go-test", []string{"go test ./..."}, plan.CatTest, false)
}

func TestBuildPlanEmpty(t *testing.T) {
	pl := BuildPlan(profile.Profile{})
	if len(pl.Steps) != 0 {
		t.Fatalf("expected empty plan, got %v", stepIDs(pl))
	}
}

func TestBuildPlanJava(t *testing.T) {
	tests := []struct {
		name string
		prof profile.Profile
		want map[string][]string
	}{
		{
			name: "maven with wrapper",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "java", VersionHint: "17", Confidence: 0.95}},
				PackageManager: "maven",
				Signals: []profile.Signal{
					{Source: "pom.xml", Key: "language", Value: "java", Confidence: 0.95},
					{Source: "pom.xml", Key: "build_tool", Value: "maven", Confidence: 0.95},
					{Source: "mvnw", Key: "wrapper", Value: "mvnw", Confidence: 0.95},
				},
			},
			want: map[string][]string{
				"java-test":  {"./mvnw -B test"},
				"java-build": {"./mvnw -B package -DskipTests"},
			},
		},
		{
			name: "maven without wrapper",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "java", Confidence: 0.95}},
				Signals: []profile.Signal{
					{Source: "pom.xml", Key: "language", Value: "java", Confidence: 0.95},
					{Source: "pom.xml", Key: "build_tool", Value: "maven", Confidence: 0.95},
				},
			},
			want: map[string][]string{
				"java-test":  {"mvn -B test"},
				"java-build": {"mvn -B package -DskipTests"},
			},
		},
		{
			name: "gradle with wrapper",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "java", VersionHint: "21", Confidence: 0.9}},
				PackageManager: "gradle",
				Signals: []profile.Signal{
					{Source: "build.gradle", Key: "language", Value: "java", Confidence: 0.9},
					{Source: "build.gradle", Key: "build_tool", Value: "gradle", Confidence: 0.9},
					{Source: "gradlew", Key: "wrapper", Value: "gradlew", Confidence: 0.95},
				},
			},
			want: map[string][]string{
				"java-test":  {"./gradlew test"},
				"java-build": {"./gradlew build -x test"},
			},
		},
		{
			name: "gradle kotlin dsl without wrapper",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "java", Confidence: 0.9}},
				Signals: []profile.Signal{
					{Source: "build.gradle.kts", Key: "language", Value: "java", Confidence: 0.9},
					{Source: "build.gradle.kts", Key: "build_tool", Value: "gradle", Confidence: 0.9},
				},
			},
			want: map[string][]string{
				"java-test":  {"gradle test"},
				"java-build": {"gradle build -x test"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := BuildPlan(tt.prof)
			for id, cmds := range tt.want {
				assertStep(t, pl, id, cmds, categoryOf(id), false)
			}
		})
	}
}

func TestBuildPlanJavaReason(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{{Name: "java", Confidence: 0.95}},
		Signals: []profile.Signal{
			{Source: "pom.xml", Key: "language", Value: "java", Confidence: 0.95},
			{Source: "mvnw", Key: "wrapper", Value: "mvnw", Confidence: 0.95},
		},
	}
	pl := BuildPlan(p)
	s := findStep(t, pl, "java-test")
	want := "pom.xml detected; mvnw wrapper"
	if s.Reason != want {
		t.Errorf("reason = %q, want %q", s.Reason, want)
	}
}

func TestBuildPlanRuby(t *testing.T) {
	tests := []struct {
		name string
		prof profile.Profile
		want map[string][]string
		omit []string
	}{
		{
			name: "rspec with rubocop",
			prof: profile.Profile{
				Languages:      []profile.Language{{Name: "ruby", VersionHint: "3.2.2", Confidence: 0.95}},
				PackageManager: "bundler",
				TestRunner:     "rspec",
				Linters:        []string{"rubocop"},
				Lockfiles:      []string{"Gemfile.lock"},
				Signals: []profile.Signal{
					{Source: "Gemfile", Key: "language", Value: "ruby", Confidence: 0.95},
					{Source: "Gemfile", Key: "test_runner", Value: "rspec", Confidence: 0.9},
				},
			},
			want: map[string][]string{
				"ruby-install": {"bundle install"},
				"ruby-lint":    {"bundle exec rubocop"},
				"ruby-test":    {"bundle exec rspec"},
			},
		},
		{
			name: "rake fallback",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "ruby", Confidence: 0.95}},
				TestRunner: "rake",
				Signals: []profile.Signal{
					{Source: "Gemfile", Key: "language", Value: "ruby", Confidence: 0.95},
					{Source: "Rakefile", Key: "test_runner", Value: "rake", Confidence: 0.85},
				},
			},
			want: map[string][]string{
				"ruby-install": {"bundle install"},
				"ruby-test":    {"bundle exec rake"},
			},
			omit: []string{"ruby-lint"},
		},
		{
			name: "no test runner declared",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "ruby", Confidence: 0.95}},
				Signals: []profile.Signal{
					{Source: "Gemfile", Key: "language", Value: "ruby", Confidence: 0.95},
				},
			},
			want: map[string][]string{"ruby-install": {"bundle install"}},
			omit: []string{"ruby-test", "ruby-lint"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := BuildPlan(tt.prof)
			for id, cmds := range tt.want {
				assertStep(t, pl, id, cmds, categoryOf(id), false)
			}
			for _, id := range tt.omit {
				assertNoStep(t, pl, id)
			}
		})
	}
}

func TestBuildPlanPHP(t *testing.T) {
	p := profile.Profile{
		Languages:      []profile.Language{{Name: "php", VersionHint: "8.1", Confidence: 0.95}},
		PackageManager: "composer",
		TestRunner:     "phpunit",
		Lockfiles:      []string{"composer.lock"},
		Signals: []profile.Signal{
			{Source: "composer.json", Key: "language", Value: "php", Confidence: 0.95},
			{Source: "phpunit.xml.dist", Key: "test_runner", Value: "phpunit", Confidence: 0.95},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "php-install", []string{"composer install"}, plan.CatInstall, false)
	assertStep(t, pl, "php-test", []string{"vendor/bin/phpunit"}, plan.CatTest, false)
	s := findStep(t, pl, "php-install")
	if want := "composer.json + composer.lock detected"; s.Reason != want {
		t.Errorf("php-install reason = %q, want %q", s.Reason, want)
	}
}

func TestBuildPlanPHPNoTests(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{{Name: "php", Confidence: 0.95}},
		Signals: []profile.Signal{
			{Source: "composer.json", Key: "language", Value: "php", Confidence: 0.95},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "php-install", []string{"composer install"}, plan.CatInstall, false)
	assertNoStep(t, pl, "php-test")
}

func TestBuildPlanDotnet(t *testing.T) {
	p := profile.Profile{
		Languages:  []profile.Language{{Name: "dotnet", VersionHint: "8.0.100", Confidence: 0.95}},
		TestRunner: "dotnet",
		Signals: []profile.Signal{
			{Source: "Demo.sln", Key: "language", Value: "dotnet", Confidence: 0.95},
			{Source: "Demo.csproj", Key: "language", Value: "dotnet", Confidence: 0.9},
			{Source: "Demo.Tests.csproj", Key: "test_runner", Value: "dotnet", Confidence: 0.9},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "dotnet-install", []string{"dotnet restore"}, plan.CatInstall, false)
	assertStep(t, pl, "dotnet-build", []string{"dotnet build --no-restore"}, plan.CatBuild, false)
	assertStep(t, pl, "dotnet-test", []string{"dotnet test --no-build"}, plan.CatTest, false)
	s := findStep(t, pl, "dotnet-install")
	if want := "Demo.sln + Demo.csproj detected"; s.Reason != want {
		t.Errorf("dotnet-install reason = %q, want %q", s.Reason, want)
	}
}

func TestBuildPlanDotnetNoTests(t *testing.T) {
	p := profile.Profile{
		Languages: []profile.Language{{Name: "dotnet", Confidence: 0.9}},
		Signals: []profile.Signal{
			{Source: "Demo.csproj", Key: "language", Value: "dotnet", Confidence: 0.9},
		},
	}
	pl := BuildPlan(p)
	assertStep(t, pl, "dotnet-install", []string{"dotnet restore"}, plan.CatInstall, false)
	assertStep(t, pl, "dotnet-build", []string{"dotnet build --no-restore"}, plan.CatBuild, false)
	assertNoStep(t, pl, "dotnet-test")
}

func TestBuildPlanCpp(t *testing.T) {
	tests := []struct {
		name string
		prof profile.Profile
		want map[string][]string
		omit []string
	}{
		{
			name: "cmake with tests",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "cpp", VersionHint: "17", Confidence: 0.95}},
				TestRunner: "ctest",
				Signals: []profile.Signal{
					{Source: "CMakeLists.txt", Key: "language", Value: "cpp", Confidence: 0.95},
					{Source: "CMakeLists.txt", Key: "build_tool", Value: "cmake", Confidence: 0.95},
					{Source: "CMakeLists.txt", Key: "test_runner", Value: "ctest", Confidence: 0.9},
				},
			},
			want: map[string][]string{
				"cpp-test":  {"ctest --test-dir build --output-on-failure"},
				"cpp-build": {"cmake -B build -DCMAKE_BUILD_TYPE=Release", "cmake --build build"},
			},
		},
		{
			name: "cmake without tests",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "cpp", Confidence: 0.95}},
				Signals: []profile.Signal{
					{Source: "CMakeLists.txt", Key: "language", Value: "cpp", Confidence: 0.95},
					{Source: "CMakeLists.txt", Key: "build_tool", Value: "cmake", Confidence: 0.95},
				},
			},
			want: map[string][]string{
				"cpp-build": {"cmake -B build -DCMAKE_BUILD_TYPE=Release", "cmake --build build"},
			},
			omit: []string{"cpp-test"},
		},
		{
			name: "makefile with test target",
			prof: profile.Profile{
				Languages:  []profile.Language{{Name: "cpp", Confidence: 0.7}},
				TestRunner: "make test",
				Signals: []profile.Signal{
					{Source: "Makefile", Key: "language", Value: "cpp", Confidence: 0.7},
					{Source: "Makefile", Key: "build_tool", Value: "make", Confidence: 0.7},
					{Source: "Makefile", Key: "test_runner", Value: "make test", Confidence: 0.7},
				},
			},
			want: map[string][]string{
				"cpp-test":  {"make test"},
				"cpp-build": {"make"},
			},
		},
		{
			name: "makefile with check target",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "cpp", Confidence: 0.7}},
				Signals: []profile.Signal{
					{Source: "Makefile", Key: "language", Value: "cpp", Confidence: 0.7},
					{Source: "Makefile", Key: "build_tool", Value: "make", Confidence: 0.7},
					{Source: "Makefile", Key: "test_runner", Value: "make check", Confidence: 0.7},
				},
			},
			want: map[string][]string{
				"cpp-test":  {"make check"},
				"cpp-build": {"make"},
			},
		},
		{
			name: "makefile without test target",
			prof: profile.Profile{
				Languages: []profile.Language{{Name: "cpp", Confidence: 0.7}},
				Signals: []profile.Signal{
					{Source: "Makefile", Key: "language", Value: "cpp", Confidence: 0.7},
					{Source: "Makefile", Key: "build_tool", Value: "make", Confidence: 0.7},
				},
			},
			want: map[string][]string{"cpp-build": {"make"}},
			omit: []string{"cpp-test"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := BuildPlan(tt.prof)
			for id, cmds := range tt.want {
				assertStep(t, pl, id, cmds, categoryOf(id), false)
			}
			for _, id := range tt.omit {
				assertNoStep(t, pl, id)
			}
		})
	}
}
