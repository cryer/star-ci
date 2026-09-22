package analyzer_test

import (
	"testing"

	"github.com/cryer/star-ci/internal/analyzer"
	"github.com/cryer/star-ci/internal/profile"
)

func assertWorkspaces(t *testing.T, prof profile.Profile, want ...profile.Workspace) {
	t.Helper()
	if len(prof.Workspaces) != len(want) {
		t.Fatalf("Workspaces = %v, want %v", prof.Workspaces, want)
	}
	for i, w := range want {
		if prof.Workspaces[i] != w {
			t.Errorf("Workspaces[%d] = %v, want %v", i, prof.Workspaces[i], w)
		}
		if !hasSignal(prof, "workspace", w.Kind+":"+w.Path) {
			t.Errorf("missing workspace signal for %s:%s", w.Kind, w.Path)
		}
	}
}

func TestWorkspacesNodeGlob(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "ws-node"))
	if err != nil {
		t.Fatal(err)
	}
	// packages/broken has no package.json and apps/ does not exist:
	// both must be skipped. Sorted by path.
	assertWorkspaces(t, prof,
		profile.Workspace{Path: "packages/api", Kind: "node"},
		profile.Workspace{Path: "packages/web", Kind: "node"},
	)
}

func TestWorkspacesPnpm(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "ws-pnpm"))
	if err != nil {
		t.Fatal(err)
	}
	// the missing/* glob matches nothing and must not error.
	assertWorkspaces(t, prof,
		profile.Workspace{Path: "apps/api", Kind: "node"},
		profile.Workspace{Path: "apps/web", Kind: "node"},
	)
}

func TestWorkspacesCargo(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "ws-cargo"))
	if err != nil {
		t.Fatal(err)
	}
	// crates/empty has no Cargo.toml, tools/missing does not exist.
	assertWorkspaces(t, prof,
		profile.Workspace{Path: "crates/alpha", Kind: "cargo"},
		profile.Workspace{Path: "crates/beta", Kind: "cargo"},
	)
}

func TestWorkspacesGoWork(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "ws-go"))
	if err != nil {
		t.Fatal(err)
	}
	// svc/gone is listed in go.work but does not exist.
	assertWorkspaces(t, prof,
		profile.Workspace{Path: "svc/one", Kind: "go"},
		profile.Workspace{Path: "svc/two", Kind: "go"},
	)
}

func TestWorkspacesNoneInPlainRepo(t *testing.T) {
	prof, err := analyzer.Analyze(fixture(t, "node-pnpm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(prof.Workspaces) != 0 {
		t.Errorf("Workspaces = %v, want none", prof.Workspaces)
	}
}
