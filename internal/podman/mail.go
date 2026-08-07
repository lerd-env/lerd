package podman

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/geodro/lerd/internal/config"
)

// mailSMTPPort is the port a mail catcher accepts SMTP on inside the lerd
// network. Mailpit and the MailHog it stands in for both listen there, and it
// is the port every framework definition already wires into a project's env.
const mailSMTPPort = 1025

// MailIni returns the conf.d ini that points PHP's mail() at the mail catcher.
// The FPM image's /usr/sbin/sendmail is BusyBox's, which connects to
// 127.0.0.1:25 unless told otherwise and finds nothing listening in the
// container, so mail() returns false and a framework that sends through it
// reports its own version of "could not send". Naming the catcher with -S is
// all it takes, and it needs no framework knowledge: Drupal, WordPress and any
// plain mail() call are fixed by the same line.
func MailIni() string {
	host := mailCatcherHost()
	return fmt.Sprintf(`; lerd: route PHP's mail() to the mail catcher lerd runs.
;
; The image's sendmail is BusyBox's, which talks to 127.0.0.1:25 by default and
; reaches nothing inside the container. Written by lerd; edit the shared or
; per-version php.ini instead, either of which overrides this.
sendmail_path = "/usr/sbin/sendmail -t -i -S %s:%s"
`, host, strconv.Itoa(mailSMTPPort))
}

// mailCatcherHost is the container the mail goes to: whichever installed service
// plays the mailpit role, so a drop-in standing in for it is used when that is
// what the machine runs. Falls back to the default stack's own name, which
// leaves an install with no catcher exactly as it was.
func mailCatcherHost() string {
	if hosts := config.ServicesInFamily("mailpit"); len(hosts) > 0 {
		return hosts[0]
	}
	return "lerd-mailpit"
}

// EnsureMailAssets writes the mail conf.d ini to its host path so the FPM
// quadlet's always-present volume has a regular file (not a podman-auto-created
// directory) at the bind-mount source. Idempotent, and rewritten only when the
// resolved catcher changes, so a reconcile doesn't churn the file.
func EnsureMailAssets() error {
	path := config.MailIniFile()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating mail ini dir: %w", err)
	}
	ini := MailIni()
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				return fmt.Errorf("removing stale mail ini directory %s: %w", path, rmErr)
			}
		} else if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == ini {
			return nil
		}
	}
	return os.WriteFile(path, []byte(ini), 0644)
}
