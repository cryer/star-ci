package profile

import (
	"sort"
	"strings"
)

// SortWorkspaces orders workspaces by Path for deterministic output.
func (p *Profile) SortWorkspaces() {
	sort.SliceStable(p.Workspaces, func(i, j int) bool {
		return p.Workspaces[i].Path < p.Workspaces[j].Path
	})
}

// AffectedWorkspaces maps a list of repo-relative changed file paths to the
// workspaces that contain at least one of them. A changed file that lies
// outside every workspace directory (root lockfiles, root manifests, shared
// code) affects all workspaces: the shared foundation may have moved, so
// everything must be re-checked. The result keeps the input order of
// workspaces, deduplicated.
func AffectedWorkspaces(workspaces []Workspace, changedFiles []string) []Workspace {
	if len(changedFiles) == 0 {
		return nil
	}
	affected := make([]bool, len(workspaces))
	all := false
	for _, f := range changedFiles {
		f = strings.TrimPrefix(strings.ReplaceAll(f, "\\", "/"), "./")
		if f == "" {
			continue
		}
		matched := false
		for i, ws := range workspaces {
			if f == ws.Path || strings.HasPrefix(f, ws.Path+"/") {
				affected[i] = true
				matched = true
			}
		}
		if !matched {
			all = true
		}
	}
	var out []Workspace
	for i, ws := range workspaces {
		if all || affected[i] {
			out = append(out, ws)
		}
	}
	return out
}
