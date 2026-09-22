package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/star-ci/star-ci/internal/profile"
)

// detectCommon records ecosystem-agnostic facts: Dockerfile and existing CI.
func detectCommon(root string, prof *profile.Profile) {
	if fileExists(root, "Dockerfile") {
		prof.HasDockerfile = true
		prof.AddSignal("Dockerfile", "dockerfile", "true", 0.95)
	}

	workflows := matchRootFiles(filepath.Join(root, ".github", "workflows"), func(n string) bool {
		return strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml")
	})
	if len(workflows) > 0 {
		prof.ExistingCI = profile.AddUnique(prof.ExistingCI, "github-actions")
		prof.AddSignal(".github/workflows/"+workflows[0], "existing_ci", "github-actions", 0.95)
	}
	if fileExists(root, ".gitlab-ci.yml") {
		prof.ExistingCI = profile.AddUnique(prof.ExistingCI, "gitlab")
		prof.AddSignal(".gitlab-ci.yml", "existing_ci", "gitlab", 0.95)
	}
	if fileExists(root, "Jenkinsfile") {
		prof.ExistingCI = profile.AddUnique(prof.ExistingCI, "jenkins")
		prof.AddSignal("Jenkinsfile", "existing_ci", "jenkins", 0.95)
	}
	if dirExists(root, ".circleci") {
		prof.ExistingCI = profile.AddUnique(prof.ExistingCI, "circleci")
		prof.AddSignal(".circleci", "existing_ci", "circleci", 0.95)
	}
}
