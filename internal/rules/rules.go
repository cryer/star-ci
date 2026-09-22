// Package rules maps a detected profile to an ordered CI plan.
package rules

import (
	"strings"

	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
)

// BuildPlan derives CI steps from the profile. Signals and languages below
// profile.MinConfidence are ignored.
func BuildPlan(p profile.Profile) plan.Plan {
	var steps []plan.Step
	for _, l := range p.Languages {
		if l.Confidence < profile.MinConfidence {
			continue
		}
		switch l.Name {
		case "node":
			steps = append(steps, nodeSteps(p)...)
		case "python":
			steps = append(steps, pythonSteps(p)...)
		case "go":
			steps = append(steps, goSteps(p)...)
		}
	}
	steps = append(steps, securitySteps(p)...)
	if p.HasDockerfile {
		steps = append(steps, plan.Step{
			ID:       "docker-build",
			Name:     "Build Docker image",
			Category: plan.CatBuild,
			Commands: []string{"docker build ."},
			Reason:   "Dockerfile detected",
			Optional: true,
		})
	}
	pl := plan.Plan{Steps: steps, Profile: p}
	pl.Sort()
	return pl
}

func nodeSteps(p profile.Profile) []plan.Step {
	pm := p.PackageManager
	if pm == "" {
		pm = "npm"
	}
	reason := nodeReason(p)
	var steps []plan.Step

	installCmd := "npm ci"
	switch pm {
	case "pnpm":
		installCmd = "pnpm install --frozen-lockfile"
	case "yarn":
		installCmd = "yarn install --frozen-lockfile"
	case "bun":
		installCmd = "bun install --frozen-lockfile"
	}
	steps = append(steps, plan.Step{
		ID:       "node-install",
		Name:     "Install dependencies (" + pm + ")",
		Category: plan.CatInstall,
		Commands: []string{installCmd},
		Reason:   reason,
	})

	var lintCmds []string
	for _, l := range p.Linters {
		switch l {
		case "eslint":
			if hasScript(p, "lint") {
				lintCmds = append(lintCmds, pm+" run lint")
			} else {
				lintCmds = append(lintCmds, "npx eslint .")
			}
		case "biome":
			lintCmds = append(lintCmds, "npx biome check .")
		}
	}
	if len(lintCmds) > 0 {
		steps = append(steps, plan.Step{
			ID:       "node-lint",
			Name:     "Lint (" + strings.Join(p.Linters, ", ") + ")",
			Category: plan.CatLint,
			Commands: lintCmds,
			Reason:   reason + "; linter " + strings.Join(p.Linters, ", ") + " configured",
		})
	}

	if contains(p.Formatters, "prettier") &&
		(hasScript(p, "format", "format:check", "check") || hasSignalValue(p, "prettier")) {
		steps = append(steps, plan.Step{
			ID:       "node-format",
			Name:     "Check formatting (prettier)",
			Category: plan.CatLint,
			Commands: []string{"npx prettier --check ."},
			Reason:   reason + "; prettier configured",
		})
	}

	if p.Typecheck == "tsc" {
		cmd := "npx tsc --noEmit"
		if hasScript(p, "typecheck") {
			cmd = "npm run typecheck"
		}
		steps = append(steps, plan.Step{
			ID:       "node-typecheck",
			Name:     "Typecheck (tsc)",
			Category: plan.CatTypecheck,
			Commands: []string{cmd},
			Reason:   reason + "; TypeScript detected",
		})
	}

	if p.TestRunner != "" || hasScript(p, "test") {
		steps = append(steps, plan.Step{
			ID:       "node-test",
			Name:     "Run tests (" + pm + ")",
			Category: plan.CatTest,
			Commands: []string{pm + " test"},
			Reason:   reason + "; test script detected",
		})
	}

	if hasScript(p, "build") {
		steps = append(steps, plan.Step{
			ID:       "node-build",
			Name:     "Build (" + pm + ")",
			Category: plan.CatBuild,
			Commands: []string{pm + " run build"},
			Reason:   reason + "; build script detected",
		})
	}
	return steps
}

func pythonSteps(p profile.Profile) []plan.Step {
	reason := pythonReason(p)
	var steps []plan.Step

	installCmd := "pip install -e ."
	switch p.PackageManager {
	case "uv":
		if hasSource(p, "pyproject.toml") || hasLockfile(p, "uv.lock") {
			installCmd = "uv sync"
		} else {
			installCmd = "uv pip install -r requirements.txt"
		}
	case "poetry":
		installCmd = "poetry install"
	default:
		if hasSource(p, "requirements.txt") {
			installCmd = "pip install -r requirements.txt"
		}
	}
	steps = append(steps, plan.Step{
		ID:       "python-install",
		Name:     "Install dependencies (python)",
		Category: plan.CatInstall,
		Commands: []string{installCmd},
		Reason:   reason,
	})

	if contains(p.Linters, "ruff") {
		steps = append(steps, plan.Step{
			ID:       "python-lint",
			Name:     "Lint (ruff)",
			Category: plan.CatLint,
			Commands: []string{"ruff check ."},
			Reason:   reason + "; ruff configured",
		})
	}

	if contains(p.Formatters, "black") {
		steps = append(steps, plan.Step{
			ID:       "python-format",
			Name:     "Check formatting (black)",
			Category: plan.CatLint,
			Commands: []string{"black --check ."},
			Reason:   reason + "; black configured",
		})
	}

	if p.Typecheck == "mypy" {
		steps = append(steps, plan.Step{
			ID:       "python-typecheck",
			Name:     "Typecheck (mypy)",
			Category: plan.CatTypecheck,
			Commands: []string{"mypy ."},
			Reason:   reason + "; mypy configured",
		})
	}

	if p.TestRunner == "pytest" {
		steps = append(steps, plan.Step{
			ID:       "python-test",
			Name:     "Run tests (pytest)",
			Category: plan.CatTest,
			Commands: []string{"python -m pytest"},
			Reason:   reason + "; pytest detected",
		})
	}
	return steps
}

func goSteps(p profile.Profile) []plan.Step {
	reason := goReason(p)
	steps := []plan.Step{
		{
			ID:       "go-install",
			Name:     "Download modules",
			Category: plan.CatInstall,
			Commands: []string{"go mod download"},
			Reason:   reason,
		},
	}
	if p.Typecheck == "govet" {
		steps = append(steps, plan.Step{
			ID:       "go-typecheck",
			Name:     "Vet (go vet)",
			Category: plan.CatTypecheck,
			Commands: []string{"go vet ./..."},
			Reason:   reason + "; govet configured",
		})
	}
	if contains(p.Linters, "golangci-lint") {
		steps = append(steps, plan.Step{
			ID:       "go-lint",
			Name:     "Lint (golangci-lint)",
			Category: plan.CatLint,
			Commands: []string{"golangci-lint run"},
			Reason:   reason + "; golangci-lint configured",
		})
	}
	steps = append(steps,
		plan.Step{
			ID:       "go-test",
			Name:     "Run tests (go test)",
			Category: plan.CatTest,
			Commands: []string{"go test ./..."},
			Reason:   reason,
		},
		plan.Step{
			ID:       "go-build",
			Name:     "Build (go build)",
			Category: plan.CatBuild,
			Commands: []string{"go build ./..."},
			Reason:   reason,
		},
	)
	return steps
}

// securitySteps adds always-optional audit and secret-scan steps per ecosystem.
func securitySteps(p profile.Profile) []plan.Step {
	var steps []plan.Step
	for _, l := range p.Languages {
		if l.Confidence < profile.MinConfidence {
			continue
		}
		switch l.Name {
		case "node":
			steps = append(steps, securityStep("security-audit-node",
				"Audit dependencies (npm)", "npm audit --audit-level=high", nodeReason(p)))
		case "python":
			steps = append(steps, securityStep("security-audit-python",
				"Audit dependencies (pip-audit)", "pip install pip-audit && pip-audit", pythonReason(p)))
		case "go":
			steps = append(steps, securityStep("security-audit-go",
				"Audit dependencies (govulncheck)", "go run golang.org/x/vuln/cmd/govulncheck@latest ./...", goReason(p)))
		}
	}
	if len(steps) > 0 {
		steps = append(steps, plan.Step{
			ID:       "security-secrets",
			Name:     "Scan for secrets (gitleaks)",
			Category: plan.CatSecurity,
			Commands: []string{"gitleaks detect --source ."},
			Reason:   "secret scanning recommended for every repository",
			Optional: true,
		})
	}
	return steps
}

func securityStep(id, name, cmd, reason string) plan.Step {
	return plan.Step{
		ID:       id,
		Name:     name,
		Category: plan.CatSecurity,
		Commands: []string{cmd},
		Reason:   reason + "; dependency audit",
		Optional: true,
	}
}

func nodeReason(p profile.Profile) string {
	parts := []string{"package.json"}
	for _, lf := range p.Lockfiles {
		switch lf {
		case "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb", "package-lock.json":
			parts = append(parts, lf)
		}
	}
	return joinDetected(parts)
}

func pythonReason(p profile.Profile) string {
	var parts []string
	for _, src := range []string{"pyproject.toml", "requirements.txt", "setup.py", "setup.cfg", "Pipfile"} {
		if hasSource(p, src) {
			parts = append(parts, src)
		}
	}
	for _, lf := range []string{"uv.lock", "poetry.lock", "Pipfile.lock"} {
		if hasLockfile(p, lf) {
			parts = append(parts, lf)
		}
	}
	if len(parts) == 0 {
		return "python project detected"
	}
	return joinDetected(parts)
}

func goReason(p profile.Profile) string {
	if hasSource(p, "go.mod") {
		return "go.mod detected"
	}
	return "go module detected"
}

func joinDetected(parts []string) string {
	return strings.Join(parts, " + ") + " detected"
}

func hasScript(p profile.Profile, names ...string) bool {
	for _, n := range names {
		if _, ok := p.Scripts[n]; ok {
			return true
		}
	}
	return false
}

func hasLockfile(p profile.Profile, name string) bool {
	return contains(p.Lockfiles, name)
}

func hasSource(p profile.Profile, source string) bool {
	for _, s := range p.Signals {
		if s.Source == source && s.Confidence >= profile.MinConfidence {
			return true
		}
	}
	return false
}

func hasSignalValue(p profile.Profile, value string) bool {
	for _, s := range p.Signals {
		if s.Value == value && s.Confidence >= profile.MinConfidence {
			return true
		}
	}
	return false
}

func contains(slice []string, v string) bool {
	for _, s := range slice {
		if s == v {
			return true
		}
	}
	return false
}
