package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

func detectCpp(root string, prof *profile.Profile) {
	if fileExists(root, "CMakeLists.txt") {
		detectCMake(root, prof)
		return
	}
	detectMakeCpp(root, prof)
}

// detectCMake handles CMake-based projects; CMakeLists.txt declares the
// standard toolchain (cmake/ctest), so confidence is high.
func detectCMake(root string, prof *profile.Profile) {
	content := readFile(root, "CMakeLists.txt")
	prof.AddLanguage("cpp", cmakeCXXStandard(content), 0.95)
	prof.AddSignal("CMakeLists.txt", "language", "cpp", 0.95)
	prof.AddSignal("CMakeLists.txt", "build_tool", "cmake", 0.95)

	if strings.Contains(content, "enable_testing") || strings.Contains(content, "add_test") {
		setTestRunner(prof, "ctest")
		prof.AddSignal("CMakeLists.txt", "test_runner", "ctest", 0.9)
	}
}

// detectMakeCpp handles plain-Makefile projects: without a manifest there is
// no declared standard, so sources must exist and confidence stays moderate.
func detectMakeCpp(root string, prof *profile.Profile) {
	makefile := ""
	for _, f := range []string{"Makefile", "makefile"} {
		if fileExists(root, f) {
			makefile = f
			break
		}
	}
	if makefile == "" || !hasCppSources(root) {
		return
	}
	prof.AddLanguage("cpp", "", 0.7)
	prof.AddSignal(makefile, "language", "cpp", 0.7)
	prof.AddSignal(makefile, "build_tool", "make", 0.7)

	for _, target := range []string{"test", "check"} {
		if makeHasTarget(readFile(root, makefile), target) {
			setTestRunner(prof, "make "+target)
			prof.AddSignal(makefile, "test_runner", "make "+target, 0.7)
			break
		}
	}
}

// cmakeCXXStandard extracts the C++ standard from a
// set(CMAKE_CXX_STANDARD 17) line; "" if not set.
func cmakeCXXStandard(content string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		i := strings.Index(t, "CMAKE_CXX_STANDARD")
		if i < 0 {
			continue
		}
		rest := t[i+len("CMAKE_CXX_STANDARD"):]
		// skip lookalikes such as CMAKE_CXX_STANDARD_REQUIRED
		if rest != "" && rest[0] != ' ' && rest[0] != '\t' && rest[0] != ')' {
			continue
		}
		rest = strings.TrimSpace(strings.TrimRight(rest, ")"))
		if f := strings.Fields(rest); len(f) > 0 {
			return f[0]
		}
	}
	return ""
}

// hasCppSources reports whether .c/.cc/.cpp files exist at the root or in
// src/, the evidence that a bare Makefile builds C/C++.
func hasCppSources(root string) bool {
	isSource := func(n string) bool {
		return strings.HasSuffix(n, ".c") ||
			strings.HasSuffix(n, ".cc") ||
			strings.HasSuffix(n, ".cpp")
	}
	if len(matchRootFiles(root, isSource)) > 0 {
		return true
	}
	return len(matchRootFiles(filepath.Join(root, "src"), isSource)) > 0
}

// makeHasTarget reports whether the Makefile declares a `target:` rule.
// Only column-0 lines count, so recipes and .PHONY lines never match.
func makeHasTarget(content, target string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, target+":") {
			return true
		}
	}
	return false
}
