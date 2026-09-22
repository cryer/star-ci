package analyzer

import (
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

func detectRuby(root string, prof *profile.Profile) {
	if !fileExists(root, "Gemfile") {
		return
	}
	gemfile := readFile(root, "Gemfile")

	hint := ""
	if fileExists(root, ".ruby-version") {
		hint = cleanVersion(readFile(root, ".ruby-version"))
	}
	if hint == "" {
		hint = gemfileRubyVersion(gemfile)
	}
	prof.AddLanguage("ruby", hint, 0.95)
	prof.AddSignal("Gemfile", "language", "ruby", 0.95)

	if prof.PackageManager == "" {
		prof.PackageManager = "bundler"
	}
	prof.AddSignal("Gemfile", "package_manager", "bundler", 0.9)

	if fileExists(root, "Gemfile.lock") {
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, "Gemfile.lock")
	}

	if gemfileHasGem(gemfile, "rspec") {
		setTestRunner(prof, "rspec")
		prof.AddSignal("Gemfile", "test_runner", "rspec", 0.9)
	} else if fileExists(root, "Rakefile") {
		setTestRunner(prof, "rake")
		prof.AddSignal("Rakefile", "test_runner", "rake", 0.85)
	}

	// rubocop is opt-in: only when the project carries explicit config.
	for _, f := range []string{".rubocop.yml", ".rubocop.yaml"} {
		if fileExists(root, f) {
			addLinter(prof, "rubocop", f, 0.95)
			break
		}
	}
}

// gemfileHasGem reports whether the Gemfile declares the given gem, matching
// the exact quoted name so "rspec-rails" does not count as "rspec".
func gemfileHasGem(content, gem string) bool {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "gem ") &&
			(strings.Contains(t, `"`+gem+`"`) || strings.Contains(t, `'`+gem+`'`)) {
			return true
		}
	}
	return false
}

// gemfileRubyVersion extracts the version from a `ruby "3.2.2"` directive.
func gemfileRubyVersion(content string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "ruby ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(t, "ruby "))
		q := strings.IndexAny(rest, `"'`)
		if q < 0 {
			continue
		}
		if end := strings.Index(rest[q+1:], string(rest[q])); end >= 0 {
			return cleanVersion(rest[q+1 : q+1+end])
		}
	}
	return ""
}
