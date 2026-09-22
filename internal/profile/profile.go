// Package profile defines the ProjectProfile: the result of scanning a
// repository for technology-stack signals. It is the single contract that
// flows from the analyzer into rules, planner, runner and renderer.
package profile

import "sort"

// Confidence threshold: signals below this are not acted upon.
const MinConfidence = 0.5

// Signal is one piece of detected evidence, kept for explainability.
type Signal struct {
	Source     string  // file or place that produced the signal, e.g. "package.json"
	Key        string  // what was detected, e.g. "package_manager", "test_runner"
	Value      string  // detected value, e.g. "pnpm", "vitest"
	Confidence float64 // 0..1
}

// Language is a detected programming language ecosystem.
type Language struct {
	Name        string  // "node" | "python" | "go"
	VersionHint string  // e.g. "20", "3.11", "1.22"; empty if unknown
	Confidence  float64 // 0..1
}

// Profile is the project portrait produced by the analyzer.
type Profile struct {
	Languages      []Language        // detected ecosystems, highest confidence first
	PackageManager string            // pnpm|npm|yarn|bun|pip|poetry|uv|go, "" if none
	TestRunner     string            // vitest|jest|mocha|pytest|unittest|go, "" if none
	Linters        []string          // eslint|biome|ruff|golangci-lint|... (only if configured)
	Formatters     []string          // prettier|black|gofmt|... (only if configured)
	Typecheck      string            // tsc|mypy|govet, "" if none
	HasDockerfile  bool              // a Dockerfile exists at the root
	ExistingCI     []string          // e.g. "github-actions"
	Lockfiles      []string          // relative paths of lockfiles found
	Scripts        map[string]string // named scripts found in manifests (e.g. package.json scripts)
	Signals        []Signal          // full evidence trail for explainability
}

// AddSignal records a piece of evidence.
func (p *Profile) AddSignal(source, key, value string, confidence float64) {
	p.Signals = append(p.Signals, Signal{Source: source, Key: key, Value: value, Confidence: confidence})
}

// AddLanguage registers a detected language, keeping the highest confidence.
func (p *Profile) AddLanguage(name, versionHint string, confidence float64) {
	for i, l := range p.Languages {
		if l.Name == name {
			if confidence > l.Confidence {
				p.Languages[i].Confidence = confidence
				p.Languages[i].VersionHint = versionHint
			}
			return
		}
	}
	p.Languages = append(p.Languages, Language{Name: name, VersionHint: versionHint, Confidence: confidence})
}

// SortLanguages orders languages by descending confidence.
func (p *Profile) SortLanguages() {
	sort.SliceStable(p.Languages, func(i, j int) bool {
		return p.Languages[i].Confidence > p.Languages[j].Confidence
	})
}

// HasLanguage reports whether a language was detected above MinConfidence.
func (p *Profile) HasLanguage(name string) bool {
	for _, l := range p.Languages {
		if l.Name == name && l.Confidence >= MinConfidence {
			return true
		}
	}
	return false
}

// LanguageVersion returns the version hint for a detected language, "" if none.
func (p *Profile) LanguageVersion(name string) string {
	for _, l := range p.Languages {
		if l.Name == name {
			return l.VersionHint
		}
	}
	return ""
}

// AddUnique appends v to the slice if not already present.
func AddUnique(slice []string, v string) []string {
	for _, s := range slice {
		if s == v {
			return slice
		}
	}
	return append(slice, v)
}
