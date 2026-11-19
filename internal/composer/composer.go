// Package composer holds settings shared by every code path that runs composer
// inside a lerd FPM container — the `lerd composer` CLI and the MCP composer
// tools alike — so they stay consistent.
package composer

import (
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// DefaultProcessTimeout raises composer's per-process timeout from its 300s
// default to 30 minutes. Composer kills any script that outruns this, and the
// post-autoload-dump `package:discover` boots the whole application, which on a
// container filesystem (cold opcache, bind-mounted vendor/) can legitimately run
// well past 300s — long enough that an otherwise-successful `composer require`
// dies mid-script. See geodro/lerd#449.
const DefaultProcessTimeout = "1800"

// ProcessTimeoutEnv returns the COMPOSER_PROCESS_TIMEOUT `KEY=VALUE` entry to
// inject into the container exec. A non-empty host value always wins, so users
// keep full control (including `0` to disable the timeout entirely); otherwise
// lerd applies its higher default in place of composer's stock 300s.
func ProcessTimeoutEnv() string {
	if v, ok := os.LookupEnv("COMPOSER_PROCESS_TIMEOUT"); ok && v != "" {
		return "COMPOSER_PROCESS_TIMEOUT=" + v
	}
	return "COMPOSER_PROCESS_TIMEOUT=" + DefaultProcessTimeout
}

// PharPath returns lerd's own composer, the phar under lerd's bin dir that
// every lerd code path runs with the container's PHP. The FPM image carries a
// composer of its own, but it is frozen at image build time and an existing tag
// is never rebuilt, where a bumped pin reaches the phar through the tools
// manifest within a day, so the phar is the one lerd runs and the image's copy
// is there for a shell opened inside the container.
func PharPath() string {
	return filepath.Join(config.BinDir(), "composer.phar")
}

// MemoryLimitEnv returns COMPOSER_MEMORY_LIMIT from the framework's cli_ini
// memory_limit: Flex's auto-scripts start php themselves, out of reach of lerd's
// -d, and forward only this. A non-empty host value wins.
func MemoryLimitEnv(fw *config.Framework) []string {
	if v := os.Getenv("COMPOSER_MEMORY_LIMIT"); v != "" {
		return []string{"COMPOSER_MEMORY_LIMIT=" + v}
	}
	if fw == nil || config.ValidatePHPIni(fw.PHP.CLIIni) != nil {
		return nil
	}
	if v := fw.PHP.CLIIni["memory_limit"]; v != "" {
		return []string{"COMPOSER_MEMORY_LIMIT=" + v}
	}
	return nil
}

// Env returns the environment every composer run for the project at dir gets.
func Env(dir string) []string {
	return append([]string{ProcessTimeoutEnv()}, MemoryLimitEnv(frameworkForDir(dir))...)
}

func frameworkForDir(dir string) *config.Framework {
	name, ok := config.DetectFrameworkForDir(dir)
	if !ok {
		return nil
	}
	fw, _ := config.GetFrameworkForDir(name, dir)
	return fw
}
