// Package analyzer detects the technology stack of a repository by
// scanning well-known manifest and config files at the repo root.
package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

// Analyze scans the repository at root and merges the results of the
// per-ecosystem detectors into a single profile.
func Analyze(root string) (profile.Profile, error) {
	var prof profile.Profile
	info, err := os.Stat(root)
	if err != nil {
		return prof, err
	}
	if !info.IsDir() {
		return prof, fmt.Errorf("not a directory: %s", root)
	}
	detectNode(root, &prof)
	detectPython(root, &prof)
	detectGo(root, &prof)
	detectCommon(root, &prof)
	prof.SortLanguages()
	return prof, nil
}

func fileExists(root, rel string) bool {
	info, err := os.Stat(filepath.Join(root, rel))
	return err == nil && !info.IsDir()
}

func dirExists(root, rel string) bool {
	info, err := os.Stat(filepath.Join(root, rel))
	return err == nil && info.IsDir()
}

func readFile(root, rel string) string {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return ""
	}
	return string(data)
}

// matchRootFiles lists root-level regular files whose name satisfies
// match, sorted for deterministic output.
func matchRootFiles(root string, match func(name string) bool) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && match(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// cleanVersion strips range operators and decoration from a version
// constraint, keeping the first concrete version: ">=3.11, <3.13" -> "3.11".
func cleanVersion(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ","); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "^~>=<!v ")
	return s
}
