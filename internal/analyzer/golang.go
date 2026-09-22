package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

var goSkipDirs = map[string]bool{".git": true, "vendor": true, "node_modules": true}

func detectGo(root string, prof *profile.Profile) {
	if !fileExists(root, "go.mod") {
		return
	}

	hint := ""
	for _, line := range strings.Split(readFile(root, "go.mod"), "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "go ") {
			hint = strings.TrimSpace(strings.TrimPrefix(t, "go "))
			break
		}
	}
	prof.AddLanguage("go", hint, 0.98)
	prof.AddSignal("go.mod", "language", "go", 0.98)

	if prof.PackageManager == "" {
		prof.PackageManager = "go"
	}
	prof.AddSignal("go.mod", "package_manager", "go", 0.95)

	if fileExists(root, "go.sum") {
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, "go.sum")
	}

	if testFile := findGoTestFile(root); testFile != "" {
		setTestRunner(prof, "go")
		prof.AddSignal(testFile, "test_runner", "go", 0.9)
	}

	for _, f := range []string{".golangci.yml", ".golangci.yaml"} {
		if fileExists(root, f) {
			addLinter(prof, "golangci-lint", f, 0.95)
			break
		}
	}

	setTypecheck(prof, "govet", "go.mod", 0.9)
}

// findGoTestFile returns the repo-relative path of the first *_test.go
// file, skipping vendor and VCS directories; "" if none.
func findGoTestFile(root string) string {
	found := ""
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return err
		}
		if info.IsDir() {
			if path != root && goSkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			if rel, err := filepath.Rel(root, path); err == nil {
				found = rel
			}
		}
		return nil
	})
	return found
}
