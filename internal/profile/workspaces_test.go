package profile

import (
	"reflect"
	"testing"
)

func ws(paths ...string) []Workspace {
	var out []Workspace
	for _, p := range paths {
		out = append(out, Workspace{Path: p, Kind: "node"})
	}
	return out
}

func wsPaths(list []Workspace) []string {
	var out []string
	for _, w := range list {
		out = append(out, w.Path)
	}
	return out
}

func TestAffectedWorkspaces(t *testing.T) {
	workspaces := ws("packages/api", "packages/web")

	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"no changes", nil, nil},
		{"single workspace", []string{"packages/web/src/index.ts"}, []string{"packages/web"}},
		{"both workspaces", []string{"packages/api/main.go", "packages/web/app.tsx"},
			[]string{"packages/api", "packages/web"}},
		{"duplicate hits", []string{"packages/web/a.ts", "packages/web/b.ts"}, []string{"packages/web"}},
		{"root lockfile affects all", []string{"pnpm-lock.yaml"},
			[]string{"packages/api", "packages/web"}},
		{"root manifest affects all", []string{"package.json", "packages/web/x.ts"},
			[]string{"packages/api", "packages/web"}},
		{"non-workspace dir affects all", []string{"docs/guide.md"},
			[]string{"packages/api", "packages/web"}},
		{"nested path still matches", []string{"packages/api/internal/db/q.go"},
			[]string{"packages/api"}},
		{"prefix without slash does not match", []string{"packages/web-extra/x.ts"},
			[]string{"packages/api", "packages/web"}},
		{"backslashes normalized", []string{`packages\web\src\index.ts`}, []string{"packages/web"}},
		{"leading dot-slash stripped", []string{"./packages/api/main.go"}, []string{"packages/api"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wsPaths(AffectedWorkspaces(workspaces, tc.changed))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("AffectedWorkspaces(%v) = %v, want %v", tc.changed, got, tc.want)
			}
		})
	}
}
