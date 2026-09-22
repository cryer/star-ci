package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestChangedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")

	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "add a")

	got, err := ChangedFiles(context.Background(), dir, "HEAD~1")
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	if want := []string{"pkg/a.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedFiles = %v, want %v", got, want)
	}

	if _, err := ChangedFiles(context.Background(), dir, "no-such-ref"); err == nil {
		t.Error("expected error for unknown ref")
	}
}
