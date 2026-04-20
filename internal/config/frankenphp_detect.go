package config

import (
	"os"
	"path/filepath"
	"strings"
)

// FrankenPHPHint describes one reason a project looks like it's set up for
// FrankenPHP. Multiple hints can be present at once.
type FrankenPHPHint struct {
	// Signal is a short machine-readable tag: "laravel-octane-franken",
	// "symfony-runtime", "runtime-frankenphp", "containerfile", "caddyfile".
	Signal string
	// Reason is a one-line human-readable explanation.
	Reason string
}

// DetectFrankenPHPHints inspects a project directory for signals that it is
// intended to run under FrankenPHP. It never returns an error; an unreadable
// composer.json or missing file simply produces no hints for that signal.
// Hints are ordered by confidence (most specific first).
func DetectFrankenPHPHints(dir string) []FrankenPHPHint {
	if dir == "" {
		return nil
	}
	var hints []FrankenPHPHint

	hasOctane := ComposerHasPackage(dir, "laravel/octane")
	hasSymfonyRuntime := ComposerHasPackage(dir, "runtime/frankenphp-symfony")
	hasRuntimeFranken := ComposerHasPackage(dir, "runtime/frankenphp")

	if hasOctane {
		if server := envValue(dir, "OCTANE_SERVER"); strings.EqualFold(server, "frankenphp") {
			hints = append(hints, FrankenPHPHint{
				Signal: "laravel-octane-franken",
				Reason: "laravel/octane is installed and OCTANE_SERVER=frankenphp in .env",
			})
		}
	}
	if hasSymfonyRuntime {
		hints = append(hints, FrankenPHPHint{
			Signal: "symfony-runtime",
			Reason: "runtime/frankenphp-symfony is installed in composer.json",
		})
	}
	if hasRuntimeFranken && !hasSymfonyRuntime {
		hints = append(hints, FrankenPHPHint{
			Signal: "runtime-frankenphp",
			Reason: "runtime/frankenphp is installed in composer.json",
		})
	}
	if fromsFrankenPHP(dir) {
		hints = append(hints, FrankenPHPHint{
			Signal: "containerfile",
			Reason: "Containerfile.lerd FROMs dunglas/frankenphp",
		})
	}

	return hints
}

// envValue reads a single KEY from the project's .env. Returns "" when the
// file, the key, or the value is missing. No expansion, no quoting.
func envValue(dir, key string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		if strings.TrimSpace(line[:eq]) == key {
			v := strings.TrimSpace(line[eq+1:])
			v = strings.Trim(v, `"'`)
			return v
		}
	}
	return ""
}

// fromsFrankenPHP returns true when the project's Containerfile.lerd starts
// from a dunglas/frankenphp image.
func fromsFrankenPHP(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "Containerfile.lerd"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			continue
		}
		return strings.Contains(strings.ToLower(line), "dunglas/frankenphp")
	}
	return false
}
