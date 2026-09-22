// Package render turns a plan.Plan into a GitHub Actions workflow YAML.
package render

import (
	"fmt"
	"strings"

	"github.com/cryer/star-ci/internal/plan"
)

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

	prof := pl.Profile
	if prof.HasLanguage("node") {
		pm := prof.PackageManager
		if pm == "pnpm" {
			b.WriteString("      - name: Set up pnpm\n")
			b.WriteString("        uses: pnpm/action-setup@v4\n")
		}
		b.WriteString("      - name: Set up Node.js\n")
		b.WriteString("        uses: actions/setup-node@v4\n")
		with := setupWith(map[string]string{
			"node-version": prof.LanguageVersion("node"),
			"cache":        pm,
		})
		if with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("python") {
		b.WriteString("      - name: Set up Python\n")
		b.WriteString("        uses: actions/setup-python@v5\n")
		if with := setupWith(map[string]string{"python-version": prof.LanguageVersion("python")}); with != "" {
			b.WriteString("        with:\n")
			b.WriteString(with)
		}
	}
	if prof.HasLanguage("go") {
		b.WriteString("      - name: Set up Go\n")
		b.WriteString("        uses: actions/setup-go@v5\n")
		if with := setupWith(map[string]string{"go-version": prof.LanguageVersion("go")}); with != "" {
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
	}

	for _, s := range pl.Steps {
		fmt.Fprintf(&b, "      - name: %s\n", s.Name)
		if s.Optional {
			b.WriteString("        continue-on-error: true\n")
		}
		b.WriteString("        run: |\n")
		for _, cmd := range s.Commands {
			fmt.Fprintf(&b, "          %s\n", cmd)
		}
	}
	return []byte(b.String()), nil
}

// setupWith renders a `with:` block body (10-space indent), dropping empty
// values. Keys are fixed per call site, so a stable order is chosen here.
func setupWith(kv map[string]string) string {
	var order []string
	for _, k := range []string{"node-version", "cache", "python-version", "go-version", "toolchain"} {
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
