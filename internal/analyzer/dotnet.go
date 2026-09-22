package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

var dotnetSkipDirs = map[string]bool{".git": true, "bin": true, "obj": true, "node_modules": true}

func detectDotnet(root string, prof *profile.Profile) {
	slns := matchRootFiles(root, func(n string) bool { return strings.HasSuffix(n, ".sln") })
	csprojs := matchRootFiles(root, func(n string) bool { return strings.HasSuffix(n, ".csproj") })
	if len(slns) == 0 && len(csprojs) == 0 {
		return
	}

	hint := ""
	if fileExists(root, "global.json") {
		hint = globalJSONSDKVersion(readFile(root, "global.json"))
	}
	if hint == "" && len(csprojs) > 0 {
		hint = xmlTagValue(readFile(root, csprojs[0]), "TargetFramework")
	}

	conf := 0.9
	if len(slns) > 0 {
		conf = 0.95
	}
	prof.AddLanguage("dotnet", hint, conf)
	for _, f := range slns {
		prof.AddSignal(f, "language", "dotnet", 0.95)
	}
	for _, f := range csprojs {
		prof.AddSignal(f, "language", "dotnet", 0.9)
	}

	if prof.PackageManager == "" {
		prof.PackageManager = "nuget"
	}
	prof.AddSignal(dotnetEvidence(slns, csprojs), "package_manager", "nuget", 0.9)

	if testProj := findDotnetTestProject(root); testProj != "" {
		setTestRunner(prof, "dotnet")
		prof.AddSignal(testProj, "test_runner", "dotnet", 0.9)
	}
}

func dotnetEvidence(slns, csprojs []string) string {
	if len(slns) > 0 {
		return slns[0]
	}
	return csprojs[0]
}

// globalJSONSDKVersion extracts the pinned SDK version from global.json.
func globalJSONSDKVersion(content string) string {
	var g struct {
		SDK struct {
			Version string `json:"version"`
		} `json:"sdk"`
	}
	if err := json.Unmarshal([]byte(content), &g); err != nil {
		return ""
	}
	return g.SDK.Version
}

// findDotnetTestProject returns the repo-relative path of the first test
// project: a csproj referencing Microsoft.NET.Test.Sdk or named
// *.Tests.csproj; "" if none.
func findDotnetTestProject(root string) string {
	found := ""
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return err
		}
		if info.IsDir() {
			if path != root && dotnetSkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".csproj") {
			return nil
		}
		isTest := strings.HasSuffix(info.Name(), ".Tests.csproj")
		if !isTest {
			if data, err := os.ReadFile(path); err == nil {
				isTest = strings.Contains(string(data), "Microsoft.NET.Test.Sdk")
			}
		}
		if isTest {
			if rel, err := filepath.Rel(root, path); err == nil {
				found = rel
			}
		}
		return nil
	})
	return found
}
