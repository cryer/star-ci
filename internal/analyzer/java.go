package analyzer

import (
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

func detectJava(root string, prof *profile.Profile) {
	switch {
	case fileExists(root, "pom.xml"):
		detectMaven(root, prof)
	case fileExists(root, "build.gradle"), fileExists(root, "build.gradle.kts"):
		detectGradle(root, prof)
	}
}

func detectMaven(root string, prof *profile.Profile) {
	pom := readFile(root, "pom.xml")
	hint := xmlTagValue(pom, "maven.compiler.source")
	if hint == "" {
		hint = xmlTagValue(pom, "java.version")
	}
	prof.AddLanguage("java", hint, 0.95)
	prof.AddSignal("pom.xml", "language", "java", 0.95)
	prof.AddSignal("pom.xml", "build_tool", "maven", 0.95)

	if prof.PackageManager == "" {
		prof.PackageManager = "maven"
	}
	prof.AddSignal("pom.xml", "package_manager", "maven", 0.9)

	if fileExists(root, "mvnw") {
		prof.AddSignal("mvnw", "wrapper", "mvnw", 0.95)
	}
}

func detectGradle(root string, prof *profile.Profile) {
	src := "build.gradle"
	if !fileExists(root, src) {
		src = "build.gradle.kts"
	}
	prof.AddLanguage("java", gradleSourceCompatibility(readFile(root, src)), 0.9)
	prof.AddSignal(src, "language", "java", 0.9)
	prof.AddSignal(src, "build_tool", "gradle", 0.9)

	if prof.PackageManager == "" {
		prof.PackageManager = "gradle"
	}
	prof.AddSignal(src, "package_manager", "gradle", 0.85)

	if fileExists(root, "gradlew") {
		prof.AddSignal("gradlew", "wrapper", "gradlew", 0.95)
	}
}

// gradleSourceCompatibility extracts the sourceCompatibility version from a
// gradle build file: "sourceCompatibility = 17" or the
// JavaVersion.VERSION_17 form.
func gradleSourceCompatibility(content string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "sourceCompatibility") {
			continue
		}
		if i := strings.Index(t, "VERSION_"); i >= 0 {
			return leadingDigits(t[i+len("VERSION_"):])
		}
		if parts := strings.SplitN(t, "=", 2); len(parts) == 2 {
			return leadingDigits(strings.Trim(strings.TrimSpace(parts[1]), `"'`))
		}
	}
	return ""
}

// leadingDigits returns the leading run of digits and dots: "17.0" -> "17.0".
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	return s[:i]
}
