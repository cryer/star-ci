// star-ci: adaptive CI for GitHub repositories.
//
// Usage:
//
//	star-ci analyze  [path] [--json]   scan a repo and print the detected profile
//	star-ci run      [path]            detect + execute the CI plan locally
//	star-ci generate [path] [-o file]  detect + render a GitHub Actions workflow
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cryer/star-ci/internal/analyzer"
	"github.com/cryer/star-ci/internal/plan"
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

func buildPlan(root string) plan.Plan {
	prof, err := analyzer.Analyze(root)
	if err != nil {
		fatal("analyze: %v", err)
	}
	return rules.BuildPlan(prof)
}

func cmdAnalyze(args []string) int {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print profile as JSON")
	_ = fs.Parse(args)
	root := resolveRoot(fs.Args())

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

	fmt.Printf("repository: %s\n\n", root)
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

	pl := buildPlan(root)
	if *dryRun {
		runner.Explain(pl, os.Stdout)
		return 0
	}
	if err := runner.Run(context.Background(), root, pl, os.Stdout); err != nil {
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

	pl := buildPlan(root)
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
