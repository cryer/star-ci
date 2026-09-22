package analyzer

import (
	"regexp"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

type pyprojectInfo struct {
	requiresPython string
	sections       map[string]bool
}

var requiresPythonRe = regexp.MustCompile(`^\s*requires-python\s*=\s*"?([^"\n]+)"?`)

// parsePyproject is a minimal line-based TOML reader: it extracts the
// requires-python constraint and the presence of [tool.*] sections.
func parsePyproject(content string) pyprojectInfo {
	info := pyprojectInfo{sections: map[string]bool{}}
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			name := strings.Trim(t, "[] ")
			for _, sec := range []string{"tool.pytest", "tool.ruff", "tool.mypy", "tool.black", "tool.poetry"} {
				if name == sec || strings.HasPrefix(name, sec+".") {
					info.sections[sec] = true
				}
			}
			continue
		}
		if info.requiresPython == "" {
			if m := requiresPythonRe.FindStringSubmatch(t); m != nil {
				info.requiresPython = cleanVersion(m[1])
			}
		}
	}
	return info
}

// requirementFiles returns root-level requirements*.txt files.
func requirementFiles(root string) []string {
	return matchRootFiles(root, func(n string) bool {
		return strings.HasPrefix(n, "requirements") && strings.HasSuffix(n, ".txt")
	})
}

// requirementsContain reports the first requirements file listing pkg.
func requirementsContain(root string, files []string, pkg string) string {
	split := func(r rune) bool {
		switch r {
		case '=', '<', '>', '~', '[', ';', ' ':
			return true
		}
		return false
	}
	for _, f := range files {
		for _, line := range strings.Split(readFile(root, f), "\n") {
			fields := strings.FieldsFunc(strings.TrimSpace(line), split)
			if len(fields) > 0 && strings.EqualFold(fields[0], pkg) {
				return f
			}
		}
	}
	return ""
}

func detectPython(root string, prof *profile.Profile) {
	hasPyproject := fileExists(root, "pyproject.toml")
	hasSetupPy := fileExists(root, "setup.py")
	hasSetupCfg := fileExists(root, "setup.cfg")
	hasPipfile := fileExists(root, "Pipfile")
	reqFiles := requirementFiles(root)
	for _, f := range reqFiles {
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, f)
	}

	if !hasPyproject && !hasSetupPy && !hasSetupCfg && !hasPipfile && len(reqFiles) == 0 {
		return
	}

	var py pyprojectInfo
	if hasPyproject {
		py = parsePyproject(readFile(root, "pyproject.toml"))
		prof.AddLanguage("python", py.requiresPython, 0.95)
		prof.AddSignal("pyproject.toml", "language", "python", 0.95)
	} else {
		src, conf := "requirements.txt", 0.85
		switch {
		case hasSetupPy:
			src = "setup.py"
		case len(reqFiles) > 0:
			src = reqFiles[0]
		case hasSetupCfg:
			src, conf = "setup.cfg", 0.8
		case hasPipfile:
			src, conf = "Pipfile", 0.8
		}
		prof.AddLanguage("python", "", conf)
		prof.AddSignal(src, "language", "python", conf)
	}

	detectPythonPM(root, &py, hasPipfile, prof)

	reqFind := func(pkg string) string { return requirementsContain(root, reqFiles, pkg) }

	if py.sections["tool.pytest"] {
		setTestRunner(prof, "pytest")
		prof.AddSignal("pyproject.toml", "test_runner", "pytest", 0.95)
	} else if src := reqFind("pytest"); src != "" {
		setTestRunner(prof, "pytest")
		prof.AddSignal(src, "test_runner", "pytest", 0.95)
	} else if dirExists(root, "tests") {
		setTestRunner(prof, "pytest")
		prof.AddSignal("tests/", "test_runner", "pytest", 0.7)
	}

	switch {
	case py.sections["tool.ruff"]:
		addLinter(prof, "ruff", "pyproject.toml", 0.95)
	case fileExists(root, "ruff.toml"):
		addLinter(prof, "ruff", "ruff.toml", 0.95)
	default:
		if src := reqFind("ruff"); src != "" {
			addLinter(prof, "ruff", src, 0.8)
		}
	}

	switch {
	case py.sections["tool.black"]:
		addFormatter(prof, "black", "pyproject.toml", 0.95)
	default:
		if src := reqFind("black"); src != "" {
			addFormatter(prof, "black", src, 0.8)
		}
	}

	switch {
	case py.sections["tool.mypy"]:
		setTypecheck(prof, "mypy", "pyproject.toml", 0.9)
	case fileExists(root, "mypy.ini"):
		setTypecheck(prof, "mypy", "mypy.ini", 0.9)
	default:
		if src := reqFind("mypy"); src != "" {
			setTypecheck(prof, "mypy", src, 0.9)
		}
	}
}

func detectPythonPM(root string, py *pyprojectInfo, hasPipfile bool, prof *profile.Profile) {
	pm, pmSrc, pmConf := "pip", "pip", 0.5
	switch {
	case fileExists(root, "uv.lock"):
		pm, pmSrc, pmConf = "uv", "uv.lock", 0.95
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, "uv.lock")
	case fileExists(root, "poetry.lock"):
		pm, pmSrc, pmConf = "poetry", "poetry.lock", 0.95
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, "poetry.lock")
	case py.sections["tool.poetry"]:
		pm, pmSrc, pmConf = "poetry", "pyproject.toml", 0.9
	case hasPipfile:
		pm, pmSrc, pmConf = "pipenv", "Pipfile", 0.9
	}
	if prof.PackageManager == "" {
		prof.PackageManager = pm
	}
	prof.AddSignal(pmSrc, "package_manager", pm, pmConf)
}

func setTestRunner(prof *profile.Profile, runner string) {
	if prof.TestRunner == "" {
		prof.TestRunner = runner
	}
}

func addLinter(prof *profile.Profile, name, source string, conf float64) {
	prof.Linters = profile.AddUnique(prof.Linters, name)
	prof.AddSignal(source, "linter", name, conf)
}

func addFormatter(prof *profile.Profile, name, source string, conf float64) {
	prof.Formatters = profile.AddUnique(prof.Formatters, name)
	prof.AddSignal(source, "formatter", name, conf)
}

func setTypecheck(prof *profile.Profile, name, source string, conf float64) {
	if prof.Typecheck == "" {
		prof.Typecheck = name
	}
	prof.AddSignal(source, "typecheck", name, conf)
}
