package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ChangedFiles lists files changed between ref and HEAD using
// `git diff --name-only <ref>...HEAD`, as repo-relative paths. Git must be
// installed and root must be inside a git work tree.
func ChangedFiles(ctx context.Context, root, ref string) ([]string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("--changed-since requires git, but git was not found in PATH")
	}
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", ref+"...HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("git diff %s...HEAD: %s", ref, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("git diff %s...HEAD: %w", ref, err)
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.TrimSpace(line); f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}
