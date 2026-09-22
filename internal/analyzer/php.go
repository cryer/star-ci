package analyzer

import (
	"encoding/json"

	"github.com/cryer/star-ci/internal/profile"
)

type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

func detectPHP(root string, prof *profile.Profile) {
	if !fileExists(root, "composer.json") {
		return
	}
	var c composerJSON
	if err := json.Unmarshal([]byte(readFile(root, "composer.json")), &c); err != nil {
		prof.AddLanguage("php", "", 0.8)
		prof.AddSignal("composer.json", "language", "php", 0.8)
		return
	}

	prof.AddLanguage("php", cleanVersion(c.Require["php"]), 0.95)
	prof.AddSignal("composer.json", "language", "php", 0.95)

	if prof.PackageManager == "" {
		prof.PackageManager = "composer"
	}
	prof.AddSignal("composer.json", "package_manager", "composer", 0.9)

	if fileExists(root, "composer.lock") {
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, "composer.lock")
	}

	phpunitSrc := ""
	for _, f := range []string{"phpunit.xml", "phpunit.xml.dist"} {
		if fileExists(root, f) {
			phpunitSrc = f
			break
		}
	}
	if phpunitSrc != "" {
		setTestRunner(prof, "phpunit")
		prof.AddSignal(phpunitSrc, "test_runner", "phpunit", 0.95)
	} else if _, ok := c.RequireDev["phpunit/phpunit"]; ok {
		setTestRunner(prof, "phpunit")
		prof.AddSignal("composer.json", "test_runner", "phpunit", 0.85)
	}
}
