// star-ci: adaptive CI for GitHub repositories.
//
// Usage:
//
//	star-ci analyze  [path] [--json]   scan a repo and print the detected profile
//	star-ci run      [path]            detect + execute the CI plan locally
//	star-ci generate [path] [-o file]  detect + render a GitHub Actions workflow
//
// All commands honor an optional .star-ci.yml at the repo root:
// `confidence` overrides the detection threshold, `coverage` declares a
// required line-coverage percentage for run, `disable` drops steps by
// ID, and `append` adds custom steps. See internal/config.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryer/star-ci/internal/analyzer"
	"github.com/cryer/star-ci/internal/config"
	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
	"github.com/cryer/star-ci/internal/render"
	"github.com/cryer/star-ci/internal/rules"
	"github.com/cryer/star-ci/internal/runner"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "analyze":
		code = cmdAnalyze(os.Args[2:])
	case "run":
		code = cmdRun(os.Args[2:])
	case "generate":
		code = cmdGenerate(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		code = 2
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprintf(os.Stderr, `star-ci — adaptive CI: detect what a repo needs, then run or generate it.

usage:
  star-ci analyze  [path] [--json]   scan a repo and print the detected profile
  star-ci run      [path]            detect + execute the CI plan locally
                   [--dry-run]       print the plan without executing
  star-ci generate [path] [-o file]  detect + render a GitHub Actions workflow
                                     (default -o .github/workflows/star-ci.yml)

config: an optional .star-ci.yml at the repo root may set 'confidence'
(threshold override), 'coverage' (required line-coverage percentage for
run), 'disable' (step IDs to drop) and 'append' (custom steps). It
applies to all three commands.
`)
}

// resolveRoot turns the optional positional path arg into an absolute dir.
func resolveRoot(args []string) string {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fatal("resolve path: %v", err)
	}
	return abs
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "star-ci: "+format+"\n", a...)
	os.Exit(1)
}

// loadConfig reads the optional .star-ci.yml under root and applies the
// confidence override before any analysis runs.
func loadConfig(root string) *config.Config {
	cfg, err := config.Load(root)
	if err != nil {
		fatal("config: %v", err)
	}
	if cfg.Confidence != nil {
		profile.MinConfidence = *cfg.Confidence
	}
	return cfg
}

// configSummary renders the analyze notice, e.g.
// ".star-ci.yml (2 disabled, 1 appended, confidence 0.70)".
func configSummary(cfg *config.Config) string {
	var parts []string
	if n := len(cfg.Disable); n > 0 {
		parts = append(parts, fmt.Sprintf("%d disabled", n))
	}
	if n := len(cfg.Append); n > 0 {
		parts = append(parts, fmt.Sprintf("%d appended", n))
	}
	if cfg.Confidence != nil {
		parts = append(parts, fmt.Sprintf("confidence %.2f", *cfg.Confidence))
	}
	if cfg.Coverage != nil {
		parts = append(parts, fmt.Sprintf("coverage %.1f%%", *cfg.Coverage))
	}
	if len(parts) == 0 {
		return filepath.Base(cfg.Path)
	}
	return fmt.Sprintf("%s (%s)", filepath.Base(cfg.Path), strings.Join(parts, ", "))
}

func buildPlan(root string) (plan.Plan, *config.Config) {
	cfg := loadConfig(root)
	prof, err := analyzer.Analyze(root)
	if err != nil {
		fatal("analyze: %v", err)
	}
	pl := rules.BuildPlan(prof)
	cfg.Apply(&pl)
	return pl, cfg
}

func cmdAnalyze(args []string) int {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print profile as JSON")
	_ = fs.Parse(args)
	root := resolveRoot(fs.Args())

	cfg := loadConfig(root)
	prof, err := analyzer.Analyze(root)
	if err != nil {
		fatal("analyze: %v", err)
	}
	prof.SortLanguages()

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(prof); err != nil {
			fatal("encode: %v", err)
		}
		return 0
	}

	fmt.Printf("repository: %s\n", root)
	if cfg.Loaded() {
		fmt.Printf("config: %s\n", configSummary(cfg))
	}
	fmt.Println()
	if len(prof.Languages) == 0 {
		fmt.Println("no supported language ecosystem detected")
	}
	for _, l := range prof.Languages {
		ver := l.VersionHint
		if ver == "" {
			ver = "unknown version"
		}
		fmt.Printf("language: %-8s (%s, confidence %.2f)\n", l.Name, ver, l.Confidence)
	}
	if prof.PackageManager != "" {
		fmt.Printf("package manager: %s\n", prof.PackageManager)
	}
	if prof.TestRunner != "" {
		fmt.Printf("test runner: %s\n", prof.TestRunner)
	}
	if len(prof.Linters) > 0 {
		fmt.Printf("linters: %v\n", prof.Linters)
	}
	if prof.Typecheck != "" {
		fmt.Printf("typecheck: %s\n", prof.Typecheck)
	}
	if prof.HasDockerfile {
		fmt.Println("dockerfile: yes")
	}
	if len(prof.ExistingCI) > 0 {
		fmt.Printf("existing CI: %v\n", prof.ExistingCI)
	}
	fmt.Printf("\nevidence (%d signals):\n", len(prof.Signals))
	for _, s := range prof.Signals {
		fmt.Printf("  [%.2f] %s: %s = %s\n", s.Confidence, s.Source, s.Key, s.Value)
	}
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print the plan without executing")
	_ = fs.Parse(args)
	root := resolveRoot(fs.Args())

	pl, cfg := buildPlan(root)
	if *dryRun {
		runner.Explain(pl, os.Stdout)
		return 0
	}
	opts := runner.Options{CoverageThreshold: cfg.Coverage}
	if err := runner.Run(context.Background(), root, pl, os.Stdout, opts); err != nil {
		fmt.Fprintf(os.Stderr, "star-ci: %v\n", err)
		return 1
	}
	return 0
}

func cmdGenerate(args []string) int {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	out := fs.String("o", "", "output workflow path (default <repo>/.github/workflows/star-ci.yml)")
	force := fs.Bool("force", false, "overwrite an existing workflow file")
	_ = fs.Parse(args)
	root := resolveRoot(fs.Args())

	pl, _ := buildPlan(root)
	if len(pl.Steps) == 0 {
		fatal("nothing detected: no CI steps to generate")
	}

	outPath := *out
	if outPath == "" {
		outPath = filepath.Join(root, ".github", "workflows", "star-ci.yml")
	} else if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(root, outPath)
	}
	if _, err := os.Stat(outPath); err == nil && !*force {
		fatal("%s already exists (use --force to overwrite)", outPath)
	}

	data, err := render.WorkflowYAML(pl)
	if err != nil {
		fatal("render: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatal("mkdir: %v", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("wrote %s (%d steps)\n", outPath, len(pl.Steps))
	for _, s := range pl.Steps {
		fmt.Printf("  - %s (%s): %s\n", s.ID, s.Category, s.Reason)
	}
	return 0
}
