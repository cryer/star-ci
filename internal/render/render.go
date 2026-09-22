// Package render turns a plan.Plan into a GitHub Actions workflow YAML.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
)

// WorkspacePlan pairs a monorepo workspace directory (repo-relative) with
// the CI plan derived for that directory.
type WorkspacePlan struct {
	Path string
	Plan plan.Plan
}

// WorkflowYAML renders pl as a GitHub Actions workflow document.
func WorkflowYAML(pl plan.Plan) ([]byte, error) {
	var b strings.Builder
	b.WriteString("name: star-ci\n")
	b.WriteString("on: [push, pull_request]\n")
	b.WriteString("jobs:\n")
	b.WriteString("  ci:\n")
	b.WriteString("    runs-on: ubuntu-latest\n")
	b.WriteString("    steps:\n")
	b.WriteString("      - uses: actions/checkout@v4\n")
	writeSetupSteps(&b, pl.Profile, "")
	writeRunSteps(&b, pl.Steps, "")
	return []byte(b.String()), nil
}

// WorkflowMatrixYAML renders one workflow covering all workspace plans.
// When every workspace produced the same steps, a single job with a
// workspace matrix is emitted (each instance runs the shared steps with
// working-directory set to ${{ matrix.workspace }}). When the plans differ,
// the matrix cannot express them, so one job per workspace is rendered
// instead, with the workspace path in the job name and defaults.run.
// working-directory. Input is sorted by path for deterministic output.
func WorkflowMatrixYAML(plans []WorkspacePlan) ([]byte, error) {
	plans = sortedPlans(plans)
	if len(plans) == 0 {
		return nil, fmt.Errorf("no workspace plans to render")
	}

	var b strings.Builder
	b.WriteString("name: star-ci\n")
	b.WriteString("on: [push, pull_request]\n")
	b.WriteString("jobs:\n")
	if identicalPlans(plans) {
		b.WriteString("  ci:\n")
		b.WriteString("    runs-on: ubuntu-latest\n")
		b.WriteString("    strategy:\n")
		b.WriteString("      fail-fast: false\n")
		b.WriteString("      matrix:\n")
		b.WriteString("        workspace:\n")
		for _, wp := range plans {
			fmt.Fprintf(&b, "          - %s\n", wp.Path)
		}
		b.WriteString("    steps:\n")
		b.WriteString("      - uses: actions/checkout@v4\n")
		writeSetupSteps(&b, plans[0].Plan.Profile, "${{ matrix.workspace }}")
		writeRunSteps(&b, plans[0].Plan.Steps, "${{ matrix.workspace }}")
		return []byte(b.String()), nil
	}
	for _, wp := range plans {
		fmt.Fprintf(&b, "  ci-%s:\n", jobKeyPart(wp.Path))
		fmt.Fprintf(&b, "    name: ci (%s)\n", wp.Path)
		b.WriteString("    runs-on: ubuntu-latest\n")
		b.WriteString("    defaults:\n")
		b.WriteString("      run:\n")
		fmt.Fprintf(&b, "        working-directory: %s\n", wp.Path)
		b.WriteString("    steps:\n")
		b.WriteString("      - uses: actions/checkout@v4\n")
		writeSetupSteps(&b, wp.Plan.Profile, wp.Path)
		writeRunSteps(&b, wp.Plan.Steps, "")
	}
	return []byte(b.String()), nil
}

// sortedPlans returns a copy of plans sorted by workspace path.
func sortedPlans(plans []WorkspacePlan) []WorkspacePlan {
	out := make([]WorkspacePlan, len(plans))
	copy(out, plans)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// identicalPlans reports whether every workspace plan has the same steps
// (IDs, names, categories, commands and optionality in the same order), in
// which case a matrix job can express all of them.
func identicalPlans(plans []WorkspacePlan) bool {
	for _, wp := range plans[1:] {
		if !sameSteps(plans[0].Plan.Steps, wp.Plan.Steps) {
			return false
		}
	}
	return true
}

func sameSteps(a, b []plan.Step) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name ||
			a[i].Category != b[i].Category || a[i].Optional != b[i].Optional {
			return false
		}
		if strings.Join(a[i].Commands, "\n") != strings.Join(b[i].Commands, "\n") {
			return false
		}
	}
	return true
}

// jobKeyPart turns a workspace path into a string safe for a job key.
func jobKeyPart(path string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, path)
}

// nodeLockfiles are the lockfile names setup-node can key its cache on, in
// the fixed preference order used when several are present.
var nodeLockfiles = []string{
	"pnpm-lock.yaml", "yarn.lock", "bun.lockb", "bun.lock", "package-lock.json",
}

// matrixPrefix is the workspace prefix used in matrix jobs.
const matrixPrefix = "${{ matrix.workspace }}"

// wsPath joins a lockfile name with the workspace prefix used in monorepo
// workflows: "" for root workflows, matrixPrefix for matrix jobs, or a
// concrete path for per-workspace fallback jobs.
func wsPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

// hashArg renders the argument for hashFiles(). Expressions cannot nest
// inside ${{ ... }}, so the matrix prefix goes through format() instead of
// plain interpolation.
func hashArg(prefix, name string) string {
	if prefix == matrixPrefix {
		return "format('{0}/" + name + "', matrix.workspace)"
	}
	return "'" + wsPath(prefix, name) + "'"
}

func hasLockfile(prof profile.Profile, name string) bool {
	for _, lf := range prof.Lockfiles {
		if lf == name {
			return true
		}
	}
	return false
}

func firstLockfile(prof profile.Profile, candidates []string) string {
	for _, c := range candidates {
		if hasLockfile(prof, c) {
			return c
		}
	}
	return ""
}

// hasPipRequirements reports whether a requirements*.txt manifest was found
// (the analyzer records them in Lockfiles), so setup-python's pip cache has
// a real dependency file to key on.
func hasPipRequirements(prof profile.Profile) bool {
	for _, lf := range prof.Lockfiles {
		if strings.HasPrefix(lf, "requirements") && strings.HasSuffix(lf, ".txt") {
			return true
		}
	}
	return false
}

// javaBuildTool returns "maven" or "gradle" when detected, so setup-java's
// built-in dependency cache can be enabled.
func javaBuildTool(prof profile.Profile) string {
	for _, s := range prof.Signals {
		if s.Key == "build_tool" && s.Confidence >= profile.MinConfidence &&
			(s.Value == "maven" || s.Value == "gradle") {
			return s.Value
		}
	}
	return ""
}

// writeCacheStep renders an actions/cache@v4 step: paths and restore-keys as
// block scalars, key as a single line.
func writeCacheStep(b *strings.Builder, name string, paths []string, key string, restoreKeys []string) {
	fmt.Fprintf(b, "      - name: %s\n", name)
	b.WriteString("        uses: actions/cache@v4\n")
	b.WriteString("        with:\n")
	b.WriteString("          path: |\n")
	for _, p := range paths {
		fmt.Fprintf(b, "            %s\n", p)
	}
	fmt.Fprintf(b, "          key: %s\n", key)
	if len(restoreKeys) > 0 {
		b.WriteString("          restore-keys: |\n")
		for _, k := range restoreKeys {
			fmt.Fprintf(b, "            %s\n", k)
		}
	}
}

// writeRunSteps renders plan steps as workflow run steps. workDir, when
// non-empty, becomes the working-directory of every run step.
func writeRunSteps(b *strings.Builder, steps []plan.Step, workDir string) {
	for _, s := range steps {
		fmt.Fprintf(b, "      - name: %s\n", s.Name)
		if s.Optional {
			b.WriteString("        continue-on-error: true\n")
		}
		if workDir != "" {
			fmt.Fprintf(b, "        working-directory: %s\n", workDir)
		}
		b.WriteString("        run: |\n")
		for _, cmd := range s.Commands {
			fmt.Fprintf(b, "          %s\n", cmd)
		}
	}
}

// writeSetupSteps renders toolchain setup steps and their dependency caches.
// Ecosystems whose setup action has no built-in cache (rust, php, dotnet)
// get an explicit actions/cache@v4 step, keyed on the lockfile hash, and
// only when the lockfile was actually detected. wsPrefix is the workspace
// path prefix for monorepo workflows ("" at the repo root).
func writeSetupSteps(b *strings.Builder, prof profile.Profile, wsPrefix string) {
	if prof.HasLanguage("node") {
		pm := prof.PackageManager
		if pm == "pnpm" {
			b.WriteString("      - name: Set up pnpm\n")
			b.WriteString("        uses: pnpm/action-setup@v4\n")
		}
		b.WriteString("      - name: Set up Node.js\n")
		b.WriteString("        uses: actions/setup-node@v4\n")
		kv := map[string]string{
			"node-version": prof.LanguageVersion("node"),
			"cache":        pm,
		}
		// at the repo root setup-node finds lockfiles on its own; inside a
		// workspace it needs the explicit path
		if wsPrefix != "" {
			if lf := firstLockfile(prof, nodeLockfiles); lf != "" {
				kv["cache-dependency-path"] = wsPath(wsPrefix, lf)
			}
		}
		if with := setupWith(kv); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("python") {
		b.WriteString("      - name: Set up Python\n")
		b.WriteString("        uses: actions/setup-python@v5\n")
		kv := map[string]string{"python-version": prof.LanguageVersion("python")}
		switch {
		case hasLockfile(prof, "poetry.lock"):
			kv["cache"] = "poetry"
			if wsPrefix != "" {
				kv["cache-dependency-path"] = wsPath(wsPrefix, "poetry.lock")
			}
		case hasPipRequirements(prof):
			kv["cache"] = "pip"
			// the default dependency path is requirements.txt only; a glob
			// also covers requirements-dev.txt style manifests
			kv["cache-dependency-path"] = wsPath(wsPrefix, "requirements*.txt")
		}
		if with := setupWith(kv); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("go") {
		b.WriteString("      - name: Set up Go\n")
		b.WriteString("        uses: actions/setup-go@v5\n")
		kv := map[string]string{"go-version": prof.LanguageVersion("go")}
		if wsPrefix != "" && hasLockfile(prof, "go.sum") {
			kv["cache-dependency-path"] = wsPath(wsPrefix, "go.sum")
		}
		if with := setupWith(kv); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("rust") {
		b.WriteString("      - name: Set up Rust\n")
		b.WriteString("        uses: dtolnay/rust-toolchain@stable\n")
		if with := setupWith(map[string]string{"toolchain": prof.LanguageVersion("rust")}); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
		if hasLockfile(prof, "Cargo.lock") {
			writeCacheStep(b, "Cache Cargo",
				[]string{"~/.cargo/registry", "~/.cargo/git", wsPath(wsPrefix, "target")},
				"${{ runner.os }}-cargo-${{ hashFiles("+hashArg(wsPrefix, "Cargo.lock")+") }}",
				[]string{"${{ runner.os }}-cargo-"})
		}
	}
	if prof.HasLanguage("java") {
		b.WriteString("      - name: Set up Java\n")
		b.WriteString("        uses: actions/setup-java@v4\n")
		kv := map[string]string{
			"distribution": "temurin",
			"java-version": prof.LanguageVersion("java"),
		}
		if tool := javaBuildTool(prof); tool != "" {
			kv["cache"] = tool
		}
		if with := setupWith(kv); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("ruby") {
		b.WriteString("      - name: Set up Ruby\n")
		b.WriteString("        uses: ruby/setup-ruby@v1\n")
		if with := setupWith(map[string]string{
			"ruby-version":  prof.LanguageVersion("ruby"),
			"bundler-cache": "true",
		}); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("php") {
		b.WriteString("      - name: Set up PHP\n")
		b.WriteString("        uses: shivammathur/setup-php@v2\n")
		if with := setupWith(map[string]string{"php-version": prof.LanguageVersion("php")}); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
		if hasLockfile(prof, "composer.lock") {
			writeCacheStep(b, "Cache Composer",
				[]string{"~/.cache/composer"},
				"${{ runner.os }}-composer-${{ hashFiles("+hashArg(wsPrefix, "composer.lock")+") }}",
				[]string{"${{ runner.os }}-composer-"})
		}
	}
	if prof.HasLanguage("dotnet") {
		b.WriteString("      - name: Set up .NET\n")
		b.WriteString("        uses: actions/setup-dotnet@v4\n")
		if with := setupWith(map[string]string{"dotnet-version": prof.LanguageVersion("dotnet")}); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
		depName := "**/*.csproj"
		if hasLockfile(prof, "packages.lock.json") {
			depName = "packages.lock.json"
		}
		writeCacheStep(b, "Cache NuGet",
			[]string{"~/.nuget/packages"},
			"${{ runner.os }}-nuget-${{ hashFiles("+hashArg(wsPrefix, depName)+") }}",
			[]string{"${{ runner.os }}-nuget-"})
	}
}

// setupWith renders a `with:` block body (10-space indent), dropping empty
// values. Keys are fixed per call site, so a stable order is chosen here.
func setupWith(kv map[string]string) string {
	var order []string
	for _, k := range []string{
		"node-version", "cache", "cache-dependency-path", "python-version",
		"go-version", "toolchain",
		"distribution", "java-version", "ruby-version", "bundler-cache",
		"php-version", "dotnet-version",
	} {
		if _, ok := kv[k]; ok {
			order = append(order, k)
		}
	}
	var b strings.Builder
	for _, k := range order {
		if v := kv[k]; v != "" {
			// quote versions so YAML doesn't coerce e.g. "3.10" to 3.1
			if strings.HasSuffix(k, "-version") || k == "toolchain" {
				fmt.Fprintf(&b, "          %s: '%s'\n", k, v)
			} else {
				fmt.Fprintf(&b, "          %s: %s\n", k, v)
			}
		}
	}
	return b.String()
}
