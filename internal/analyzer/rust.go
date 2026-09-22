package analyzer

import (
	"strings"

	"github.com/cryer/star-ci/internal/profile"
)

func detectRust(root string, prof *profile.Profile) {
	if !fileExists(root, "Cargo.toml") {
		return
	}

	hint := ""
	for _, f := range []string{"rust-toolchain", "rust-toolchain.toml"} {
		if fileExists(root, f) {
			hint = rustToolchainChannel(readFile(root, f))
			break
		}
	}
	if hint == "" {
		hint = cargoRustVersion(readFile(root, "Cargo.toml"))
	}
	prof.AddLanguage("rust", hint, 0.98)
	prof.AddSignal("Cargo.toml", "language", "rust", 0.98)

	if prof.PackageManager == "" {
		prof.PackageManager = "cargo"
	}
	prof.AddSignal("Cargo.toml", "package_manager", "cargo", 0.95)

	if fileExists(root, "Cargo.lock") {
		prof.Lockfiles = profile.AddUnique(prof.Lockfiles, "Cargo.lock")
	}

	// Cargo.toml alone declares the standard toolchain: cargo test is
	// built in, no extra configuration is required.
	setTestRunner(prof, "cargo")
	prof.AddSignal("Cargo.toml", "test_runner", "cargo", 0.9)

	// clippy and rustfmt are opt-in: only when the project carries
	// explicit configuration for them.
	for _, f := range []string{"clippy.toml", ".clippy.toml"} {
		if fileExists(root, f) {
			addLinter(prof, "clippy", f, 0.95)
			break
		}
	}
	for _, f := range []string{"rustfmt.toml", ".rustfmt.toml"} {
		if fileExists(root, f) {
			addFormatter(prof, "rustfmt", f, 0.95)
			break
		}
	}
}

// rustToolchainChannel extracts the channel from a rust-toolchain file:
// either a bare channel name ("stable", "1.75.0") or the TOML form with
// a `channel = "..."` line.
func rustToolchainChannel(content string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "channel") {
			if parts := strings.SplitN(t, "=", 2); len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if t := strings.TrimSpace(line); t != "" &&
			!strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "[") {
			return t
		}
	}
	return ""
}

// cargoRustVersion extracts the rust-version (MSRV) field from Cargo.toml.
func cargoRustVersion(content string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "rust-version") {
			if parts := strings.SplitN(t, "=", 2); len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
	}
	return ""
}
