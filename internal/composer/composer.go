// Package composer holds settings shared by every code path that runs composer
// inside a lerd FPM container — the `lerd composer` CLI and the MCP composer
// tools alike — so they stay consistent.
package composer

import "os"

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
