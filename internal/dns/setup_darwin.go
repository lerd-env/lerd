//go:build darwin

package dns

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// readUpstreamDNS reads upstream DNS servers from /etc/resolv.conf.
// On macOS the OS keeps /etc/resolv.conf up-to-date with DHCP-assigned DNS servers,
// so parsing it gives the real upstreams without needing nmcli or resolvectl.
func readUpstreamDNS() []string {
	return parseNameservers("/etc/resolv.conf")
}

// defaultUpstreamFallback returns nil on macOS: pasta's 169.254.1.1 isn't
// routable from inside Podman Machine. With no fallback dnsmasq omits
// no-resolv and uses the container's /etc/resolv.conf, which podman seeds
// from the host.
func defaultUpstreamFallback() []string { return nil }

// lerdResolverContent is the exact body lerd writes into /etc/resolver/<tld>.
// It doubles as the signature for recognising lerd-owned resolver files when
// pruning, so we never delete a resolver file the user created by hand.
var lerdResolverContent = []byte("nameserver 127.0.0.1\nport 5300\n")

// resolverTLDs returns the TLDs that get an /etc/resolver/<tld> file: the
// active set (config.ActiveTLDs) minus the suffixes macOS resolves below the
// resolver layer, for which a resolver file is inert — "localhost" (loopback
// special-case) and "local" (mDNS/Bonjour). Always returns at least "test" so
// a fresh install still wires up the default.
func resolverTLDs() []string {
	var out []string
	for _, t := range config.ActiveTLDs() {
		if t == "localhost" || t == "local" {
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		out = []string{"test"}
	}
	return out
}

// ConfigureResolver writes /etc/resolver/<tld> for every active TLD so macOS
// routes those queries to the lerd-dns dnsmasq container on port 5300, then
// prunes any lerd-owned resolver file for a TLD that is no longer in use. macOS
// checks /etc/resolver/<tld> automatically — no daemon restart required.
func ConfigureResolver() error {
	tlds := resolverTLDs()

	for _, tld := range tlds {
		resolverFile := filepath.Join("/etc/resolver", tld)
		if isFileContent(resolverFile, lerdResolverContent) {
			continue
		}
		fmt.Println("  [sudo required] Configuring /etc/resolver for ." + tld + " DNS resolution")
		if err := sudoWriteFile(resolverFile, lerdResolverContent, 0644); err != nil {
			return err
		}
	}

	pruneStaleResolverFiles(tlds)
	return nil
}

// pruneStaleResolverFiles removes lerd-owned /etc/resolver/<tld> files whose
// TLD is not in keep. Only files whose content matches lerdResolverContent are
// touched, so user-authored resolver files are never deleted.
func pruneStaleResolverFiles(keep []string) {
	wanted := map[string]bool{}
	for _, t := range keep {
		wanted[t] = true
	}
	entries, err := os.ReadDir("/etc/resolver")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || wanted[e.Name()] {
			continue
		}
		path := filepath.Join("/etc/resolver", e.Name())
		if !isFileContent(path, lerdResolverContent) {
			continue
		}
		rmCmd := exec.Command("sudo", "rm", "-f", path)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run() //nolint:errcheck
	}
}

// Teardown removes every lerd-owned /etc/resolver file. It scans by content
// rather than reading the configured TLD so uninstall leaves nothing behind
// even when several TLDs were active.
func Teardown() {
	entries, err := os.ReadDir("/etc/resolver")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join("/etc/resolver", e.Name())
		if !isFileContent(path, lerdResolverContent) {
			continue
		}
		rmCmd := exec.Command("sudo", "rm", "-f", path)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run() //nolint:errcheck
	}
}

// InstallSudoers writes a sudoers drop-in granting the current user passwordless
// access to write /etc/resolver/<tld> for each active TLD. This is required so
// the DNS watcher can automatically repair the resolver config after sleep/wake
// without prompting. Re-run when the active TLD set changes (e.g. a site is
// linked on a brand-new TLD) so the new resolver path is covered.
func InstallSudoers() error {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	if user == "" {
		return fmt.Errorf("cannot determine current user")
	}

	content := renderDarwinSudoers(user, resolverTLDs())

	const sudoersPath = "/etc/sudoers.d/lerd"
	if isFileContent(sudoersPath, []byte(content)) {
		return nil
	}

	fmt.Println("  [sudo required] Installing DNS sudoers rule")
	if err := sudoWriteFile(sudoersPath, []byte(content), 0440); err != nil {
		return fmt.Errorf("writing sudoers drop-in: %w", err)
	}
	return nil
}

// renderDarwinSudoers returns the macOS sudoers content for user + every active
// TLD's resolver path. Every command argument is fully qualified — no
// wildcards — so the rules pass modern strict sudo parsers (sudo-rs on Ubuntu
// 26.04+, C sudo >= 1.9.16 on Fedora 41+ / Arch / openSUSE Tumbleweed / NixOS
// unstable). macOS bundled sudo is still permissive today but Apple is
// following upstream; writing the rules with no wildcards now avoids surprise
// breakage on a future macOS point update.
func renderDarwinSudoers(user string, tlds []string) string {
	var sb strings.Builder
	sb.WriteString("# Lerd: passwordless DNS resolver writes for /etc/resolver/<tld>.\n")
	sb.WriteString("# Rules are fully qualified with no wildcards in command\n")
	sb.WriteString("# arguments so they pass strict sudo parsers (sudo-rs, C\n")
	sb.WriteString("# sudo >= 1.9.16). The matching code path pipes content\n")
	sb.WriteString("# through `sudo tee <dest>` instead of\n")
	sb.WriteString("# `sudo cp /var/folders/.../lerd-sudo-* <dest>` for the same reason.\n")
	fmt.Fprintf(&sb, "%s ALL=(root) NOPASSWD: /bin/mkdir -p /etc/resolver\n", user)
	for _, tld := range tlds {
		resolverPath := "/etc/resolver/" + tld
		fmt.Fprintf(&sb, "%s ALL=(root) NOPASSWD: /usr/bin/tee %s\n", user, resolverPath)
		fmt.Fprintf(&sb, "%s ALL=(root) NOPASSWD: /bin/chmod 644 %s\n", user, resolverPath)
	}
	// Wildcard-free grant for the root-owned resolver helper, so the dashboard
	// can enable a brand-new ending without a password prompt. The helper takes
	// no arguments (it reads the wanted endings from stdin), so this exact-path,
	// no-args rule passes strict sudo parsers and can't be invoked with an
	// injected argument. See resolver_helper_darwin.go for the security model.
	fmt.Fprintf(&sb, "%s ALL=(root) NOPASSWD: %s\n", user, resolverHelperPath)
	return sb.String()
}

// ReadContainerDNS returns nil on macOS — the Podman network does not need
// container-side DNS servers because dnsmasq runs natively, not in a container.
func ReadContainerDNS() []string { return nil }

// ReadUpstreamDNS returns upstream DNS server IPs from /etc/resolv.conf.
func ReadUpstreamDNS() []string {
	return readUpstreamDNS()
}

// ResolverHint returns a user-facing hint for restarting DNS on macOS.
func ResolverHint() string {
	return "run 'lerd install' to reconfigure DNS"
}
