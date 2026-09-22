package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

// workspaceManifests maps a workspace kind to the manifest a directory must
// contain to count as a workspace of that kind.
var workspaceManifests = map[string]string{
	"node":  "package.json",
	"cargo": "Cargo.toml",
	"go":    "go.mod",
}

// detectWorkspaces records monorepo package directories declared by
// package.json workspaces, pnpm-workspace.yaml, Cargo.toml [workspace]
// members and go.work use directives. Glob patterns are expanded to real
// directories; entries without the kind's manifest are skipped.
func detectWorkspaces(root string, prof *profile.Profile) {
	add := func(kind, source string, patterns []string) {
		manifest := workspaceManifests[kind]
		for _, dir := range expandWorkspacePatterns(root, patterns) {
			if !fileExists(root, dir+"/"+manifest) {
				continue
			}
			ws := profile.Workspace{Path: dir, Kind: kind}
			duplicate := false
			for _, existing := range prof.Workspaces {
				if existing == ws {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			prof.Workspaces = append(prof.Workspaces, ws)
			prof.AddSignal(source, "workspace", kind+":"+dir, 0.9)
		}
	}

	if fileExists(root, "package.json") {
		add("node", "package.json", nodeWorkspacePatterns(readFile(root, "package.json")))
	}
	if fileExists(root, "pnpm-workspace.yaml") {
		add("node", "pnpm-workspace.yaml", pnpmWorkspacePatterns(readFile(root, "pnpm-workspace.yaml")))
	}
	if fileExists(root, "Cargo.toml") {
		add("cargo", "Cargo.toml", cargoWorkspaceMembers(readFile(root, "Cargo.toml")))
	}
	if fileExists(root, "go.work") {
		add("go", "go.work", goWorkUseDirs(readFile(root, "go.work")))
	}

	prof.SortWorkspaces()
}

// expandWorkspacePatterns turns workspace patterns (plain dirs or globs like
// "packages/*") into repo-relative directories that exist, sorted and
// deduplicated. Non-existent entries are dropped.
func expandWorkspacePatterns(root string, patterns []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, pat := range patterns {
		pat = strings.Trim(strings.TrimSpace(pat), `"'`)
		pat = strings.TrimPrefix(pat, "./")
		pat = strings.TrimSuffix(pat, "/")
		if pat == "" || pat == "." || strings.HasPrefix(pat, "!") || strings.HasPrefix(pat, "..") {
			continue
		}
		if strings.ContainsAny(pat, "*?[") {
			matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
			if err != nil {
				continue
			}
			for _, m := range matches {
				if info, err := os.Stat(m); err == nil && info.IsDir() {
					if rel, err := filepath.Rel(root, m); err == nil {
						addDir(rel, seen, &out)
					}
				}
			}
			continue
		}
		if dirExists(root, pat) {
			addDir(pat, seen, &out)
		}
	}
	sort.Strings(out)
	return out
}

func addDir(rel string, seen map[string]bool, out *[]string) {
	rel = filepath.ToSlash(rel)
	if rel == "." || seen[rel] {
		return
	}
	seen[rel] = true
	*out = append(*out, rel)
}

// nodeWorkspacePatterns reads the workspaces field of package.json: either a
// plain array or an object with a packages array (yarn classic form).
func nodeWorkspacePatterns(content string) []string {
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil || len(pkg.Workspaces) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(pkg.Workspaces, &list); err == nil {
		return list
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(pkg.Workspaces, &obj); err == nil {
		return obj.Packages
	}
	return nil
}

// pnpmWorkspacePatterns line-scans pnpm-workspace.yaml for the top-level
// packages: list.
func pnpmWorkspacePatterns(content string) []string {
	var out []string
	inPackages := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inPackages {
			if trimmed == "packages:" {
				inPackages = true
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			break // next top-level key ends the list
		}
		if rest, ok := strings.CutPrefix(trimmed, "-"); ok {
			if p := strings.Trim(strings.TrimSpace(rest), `"'`); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// cargoWorkspaceMembers line-scans Cargo.toml for the members list inside
// the [workspace] section, handling both inline and multi-line arrays.
func cargoWorkspaceMembers(content string) []string {
	var out []string
	inWorkspace, inMembers := false, false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if i := strings.Index(trimmed, "#"); i >= 0 {
			trimmed = strings.TrimSpace(trimmed[:i])
		}
		switch {
		case trimmed == "[workspace]":
			inWorkspace = true
		case strings.HasPrefix(trimmed, "["):
			inWorkspace = false
		}
		if !inWorkspace {
			continue
		}
		if !inMembers {
			if rest, ok := strings.CutPrefix(trimmed, "members"); ok {
				rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "="))
				if rest, ok := strings.CutPrefix(rest, "["); ok {
					inMembers = true
					out = append(out, quotedItems(rest)...)
					if strings.Contains(rest, "]") {
						inMembers = false
					}
				}
			}
			continue
		}
		out = append(out, quotedItems(trimmed)...)
		if strings.Contains(trimmed, "]") {
			inMembers = false
		}
	}
	return out
}

// quotedItems extracts double-quoted strings from a TOML array fragment.
func quotedItems(s string) []string {
	var out []string
	for {
		i := strings.Index(s, `"`)
		if i < 0 {
			return out
		}
		s = s[i+1:]
		j := strings.Index(s, `"`)
		if j < 0 {
			return out
		}
		if item := s[:j]; item != "" {
			out = append(out, item)
		}
		s = s[j+1:]
	}
}

// goWorkUseDirs line-scans go.work for use directives, both the single-line
// form ("use ./foo") and the block form ("use ( ... )").
func goWorkUseDirs(content string) []string {
	var out []string
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if i := strings.Index(trimmed, "//"); i >= 0 {
			trimmed = strings.TrimSpace(trimmed[:i])
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			if dir := strings.Trim(trimmed, `"'`); dir != "" {
				out = append(out, dir)
			}
			continue
		}
		if trimmed == "use (" || trimmed == "use(" {
			inBlock = true
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "use "); ok {
			if dir := strings.Trim(strings.TrimSpace(rest), `"'`); dir != "" {
				out = append(out, dir)
			}
		}
	}
	return out
}
