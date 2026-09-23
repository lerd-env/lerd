package cli

import (
	"errors"
	"strings"
	"testing"
)

// lerd prepends its shims dir to PATH once, at install time. A tool whose rc
// entry lands below that line takes `php` back, and the only symptom is that
// container hostnames stop resolving for CLI commands while the browser is
// fine. Doctor has to name what is in front.
func TestShimShadowFinding(t *testing.T) {
	const shim = "/home/u/.local/share/lerd/bin/php"

	cases := []struct {
		name               string
		resolved           string
		lookErr            error
		wantStatus, wantIn string
	}{
		{"lerd's shim leads", shim, nil, "ok", ""},
		{"a host php shadows it", "/opt/homebrew/bin/php", nil, "warn", "/opt/homebrew/bin/php"},
		// Not on PATH at all means the rc entry never landed, which breaks the
		// same commands for the same reason and wants the same look.
		{"nothing on PATH", "", errors.New("not found"), "warn", "not on your PATH"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, detail := shimShadowFinding("php", shim, c.resolved, c.lookErr)
			if status != c.wantStatus {
				t.Errorf("status = %q, want %q (detail %q)", status, c.wantStatus, detail)
			}
			if c.wantIn != "" && !strings.Contains(detail, c.wantIn) {
				t.Errorf("detail %q should mention %q", detail, c.wantIn)
			}
		})
	}
}

// The shadowing message is the whole value of the check, so it has to point at
// the cause rather than at the PATH in the abstract.
func TestShimShadowFindingExplainsTheSymptom(t *testing.T) {
	_, detail := shimShadowFinding("php", "/home/u/.local/share/lerd/bin/php", "/usr/bin/php", nil)
	for _, want := range []string{"host", "lerd-"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q should mention %q", detail, want)
		}
	}
}

// A Node version manager that re-prepends its own bin after lerd's PATH line
// (nvm's `use`, fnm, volta) hands back `node`, and the build then fails on a
// Node the project never targeted. The message has to name Node, not the
// container network php loses.
func TestShimShadowFindingNodeExplainsTheVersion(t *testing.T) {
	_, detail := shimShadowFinding("node", "/home/u/.local/share/lerd/bin/node", "/home/u/.nvm/versions/node/v16.20.1/bin/node", nil)
	for _, want := range []string{"/home/u/.nvm/versions/node/v16.20.1/bin/node", "Node"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q should mention %q", detail, want)
		}
	}
	if strings.Contains(detail, "lerd-") {
		t.Errorf("detail %q should not reuse the php hostname symptom", detail)
	}
}
