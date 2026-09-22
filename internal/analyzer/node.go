package analyzer

import (
	"encoding/json"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

type packageJSON struct {
	Engines         map[string]string `json:"engines"`
	PackageManager  string            `json:"packageManager"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	ESLintConfig    json.RawMessage   `json:"eslintConfig"`
	Prettier        json.RawMessage   `json:"prettier"`
}

func (p *packageJSON) hasDep(name string) bool {
	if _, ok := p.Dependencies[name]; ok {
		return true
	}
	_, ok := p.DevDependencies[name]
	return ok
}

func detectNode(root string, prof *profile.Profile) {
	if !fileExists(root, "package.json") {
		return
	}
	var pkg packageJSON
	if err := json.Unmarshal([]byte(readFile(root, "package.json")), &pkg); err != nil {
		prof.AddLanguage("node", "", 0.8)
		prof.AddSignal("package.json", "language", "node", 0.8)
		return
	}

	hint := ""
	for _, f := range []string{".nvmrc", ".node-version"} {
		if fileExists(root, f) {
			hint = cleanVersion(readFile(root, f))
			break
		}
	}
	if hint == "" {
		hint = cleanVersion(pkg.Engines["node"])
	}
	prof.AddLanguage("node", hint, 0.95)
	prof.AddSignal("package.json", "language", "node", 0.95)

	detectNodePM(root, &pkg, prof)

	if len(pkg.Scripts) > 0 {
		if prof.Scripts == nil {
			prof.Scripts = map[string]string{}
		}
		for k, v := range pkg.Scripts {
			prof.Scripts[k] = v
		}
	}

	if test, ok := pkg.Scripts["test"]; ok {
		runner, conf := "", 0.8
		for _, fw := range []string{"vitest", "jest", "mocha"} {
			if pkg.hasDep(fw) {
				runner = fw
				break
			}
		}
		if runner == "" && strings.Contains(test, "node --test") {
			runner = "node --test"
		}
		if runner == "" {
			runner, conf = "npm", 0.6
		}
		if prof.TestRunner == "" {
			prof.TestRunner = runner
		}
		prof.AddSignal("package.json", "test_runner", runner, conf)
	}

	eslintCfgs := matchRootFiles(root, func(n string) bool {
		return strings.HasPrefix(n, ".eslintrc") || strings.HasPrefix(n, "eslint.config.")
	})
	switch {
	case len(eslintCfgs) > 0:
		prof.Linters = profile.AddUnique(prof.Linters, "eslint")
		prof.AddSignal(eslintCfgs[0], "linter", "eslint", 0.95)
	case len(pkg.ESLintConfig) > 0:
		prof.Linters = profile.AddUnique(prof.Linters, "eslint")
		prof.AddSignal("package.json", "linter", "eslint", 0.95)
	case pkg.hasDep("eslint"):
		prof.Linters = profile.AddUnique(prof.Linters, "eslint")
		prof.AddSignal("package.json", "linter", "eslint", 0.7)
	}
	if fileExists(root, "biome.json") {
		prof.Linters = profile.AddUnique(prof.Linters, "biome")
		prof.AddSignal("biome.json", "linter", "biome", 0.95)
	}

	prettierCfgs := matchRootFiles(root, func(n string) bool {
		return strings.HasPrefix(n, ".prettierrc") || strings.HasPrefix(n, "prettier.config.")
	})
	switch {
	case len(prettierCfgs) > 0:
		prof.Formatters = profile.AddUnique(prof.Formatters, "prettier")
		prof.AddSignal(prettierCfgs[0], "formatter", "prettier", 0.95)
	case len(pkg.Prettier) > 0:
		prof.Formatters = profile.AddUnique(prof.Formatters, "prettier")
		prof.AddSignal("package.json", "formatter", "prettier", 0.95)
	case pkg.hasDep("prettier"):
		prof.Formatters = profile.AddUnique(prof.Formatters, "prettier")
		prof.AddSignal("package.json", "formatter", "prettier", 0.7)
	}

	if fileExists(root, "tsconfig.json") && pkg.hasDep("typescript") {
		if prof.Typecheck == "" {
			prof.Typecheck = "tsc"
		}
		prof.AddSignal("tsconfig.json", "typecheck", "tsc", 0.9)
	}

	if pkg.hasDep("next") {
		prof.AddSignal("package.json", "framework", "next", 0.9)
	}
}

// detectNodePM resolves the package manager: lockfile evidence first,
// then the packageManager field, then an npm default.
func detectNodePM(root string, pkg *packageJSON, prof *profile.Profile) {
	pm, pmSrc, pmConf := "", "", 0.0
	for _, lf := range []struct{ file, pm string }{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"bun.lockb", "bun"},
		{"bun.lock", "bun"},
		{"package-lock.json", "npm"},
	} {
		if fileExists(root, lf.file) {
			prof.Lockfiles = profile.AddUnique(prof.Lockfiles, lf.file)
			if pm == "" {
				pm, pmSrc, pmConf = lf.pm, lf.file, 0.95
			}
		}
	}
	if pm == "" && pkg.PackageManager != "" {
		name := pkg.PackageManager // e.g. "pnpm@9.1.0"
		if i := strings.Index(name, "@"); i >= 0 {
			name = name[:i]
		}
		pm, pmSrc, pmConf = name, "package.json", 0.9
	}
	if pm == "" {
		pm, pmSrc, pmConf = "npm", "package.json", 0.5
	}
	if prof.PackageManager == "" {
		prof.PackageManager = pm
	}
	prof.AddSignal(pmSrc, "package_manager", pm, pmConf)
}
