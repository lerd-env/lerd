//go:build darwin

package dns

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// resolverHelperPath is the root-owned helper that writes /etc/resolver/<tld>
// files on behalf of the unprivileged lerd-ui / watcher. It lives under /etc
// (root-owned, no spaces in the path so the sudoers grant stays simple) and is
// installed by `lerd install` / `lerd dns:apply`, both of which run with an
// interactive sudo.
const resolverHelperPath = "/etc/lerd/resolver-apply.sh"

// resolverHelperScript is the shell run as root (via a wildcard-free passwordless
// sudoers grant) to apply the per-TLD macOS resolver files without a password
// prompt — which is what lets the dashboard enable a brand-new ending on its own.
//
// SECURITY MODEL. The script is intentionally narrow so that being able to run
// it passwordlessly is low-risk:
//   - It reads the wanted endings from stdin (the caller passes the active set),
//     never from a user-writable file it execs.
//   - Each ending must be a single DNS label ([a-z0-9-], no dots), so it can
//     only ever touch /etc/resolver/<label> — never an arbitrary path.
//   - The content written is the fixed loopback→5300 mapping; nothing the caller
//     supplies ends up in the file body.
//   - A deny-list rejects real public suffixes (com, dev, io, …) plus local and
//     localhost, so a background process can NOT reroute real-internet DNS or
//     collide with the mDNS/loopback special-cases — it can only point a
//     private/dev label at this machine's own dnsmasq.
//   - Pruning only ever removes resolver files whose content is byte-for-byte
//     lerd's own, so a user's hand-written resolver files are never deleted.
//
// Residual trade-off (documented for the operator): with the helper installed,
// any process running as the user can route a *private* dev TLD to the local
// resolver without the sudo password. That is the cost of "add an ending from
// the dashboard without a terminal step"; the deny-list keeps it away from real
// domains. Installs that never run `lerd install`/`dns:apply` don't have the
// helper at all and keep the fully-manual behaviour.
func resolverHelperScript() string {
	// Keep this deny-set aligned with config.publicTLDs (advisory hardening).
	const deny = " com net org info biz dev app io co ai me tech site online " +
		"store shop xyz cloud digital studio page zip mov uk us de fr ca au in " +
		"local localhost "

	return `#!/bin/sh
# Lerd resolver helper — installed root-owned by ` + "`lerd install`" + `.
# Reads wanted endings (one per line) from stdin and writes one
# /etc/resolver/<tld> per validated ending, pruning lerd-owned resolver files
# that are no longer wanted. See internal/dns/resolver_helper_darwin.go for the
# security model. Only writes the fixed loopback->5300 mapping for single-label
# private endings; refuses real public suffixes and local/localhost.
set -eu

DENY="` + deny + `"
CONTENT='nameserver 127.0.0.1
port 5300'

mkdir -p /etc/resolver

WANTED=" "
while IFS= read -r raw; do
    tld=$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')
    # single DNS label only: [a-z0-9-], not empty, no leading/trailing hyphen
    case "$tld" in
        ''|*[!a-z0-9-]*) continue ;;
        -*|*-) continue ;;
    esac
    # refuse real public suffixes and the OS-owned specials
    case "$DENY" in *" $tld "*) continue ;; esac

    printf '%s\n' "$CONTENT" > "/etc/resolver/$tld"
    chmod 644 "/etc/resolver/$tld"
    WANTED="$WANTED$tld "
done

# Prune lerd-owned resolver files that are no longer wanted. Only files whose
# content is exactly lerd's are touched, so user-authored ones are left alone.
for f in /etc/resolver/*; do
    [ -f "$f" ] || continue
    name=$(basename "$f")
    case "$WANTED" in *" $name "*) continue ;; esac
    if [ "$(cat "$f" 2>/dev/null)" = "$CONTENT" ]; then
        rm -f "$f"
    fi
done
`
}

// InstallResolverHelper writes the root-owned helper script via an interactive
// sudo (so it must be called from a terminal context — `lerd install` or
// `lerd dns:apply`, never the background daemon). sudoWriteFile pipes through
// `sudo tee`, so the file lands root-owned; the parent /etc/lerd is created
// root-owned by the same path.
func InstallResolverHelper() error {
	return sudoWriteFile(resolverHelperPath, []byte(resolverHelperScript()), 0755)
}

// AutoApplyResolver applies the per-TLD resolver files for the current active
// set without a password prompt, by piping the wanted endings to the root-owned
// helper through the passwordless sudoers grant. It is the path the dashboard
// uses to enable a brand-new ending on its own. Returns an error (for the caller
// to fall back on) when the helper isn't installed/granted yet — e.g. on an
// install that predates the helper.
func AutoApplyResolver() error {
	tlds := resolverTLDs()
	input := strings.Join(tlds, "\n") + "\n"

	cmd := exec.Command("sudo", "-n", resolverHelperPath)
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resolver helper (%s): %w: %s", resolverHelperPath, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
