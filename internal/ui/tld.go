package ui

import (
	"fmt"
	"net/http"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/services"
)

// tldInActiveSet reports whether tld is already answered by the resolver (some
// registered site already uses it), so the caller knows if adding it introduces
// a brand-new ending.
func tldInActiveSet(tld string) bool {
	for _, t := range config.ActiveTLDs() {
		if t == tld {
			return true
		}
	}
	return false
}

// resolveTLDParam reads the optional ?tld= query parameter for a domain action,
// falling back to the global default TLD. The value is validated; a hard error
// is returned to reject (e.g. localhost with DNS on), warnings are ignored here
// (the caller surfaces its own guidance).
func resolveTLDParam(r *http.Request, cfg *config.GlobalConfig) (string, error) {
	tld := r.URL.Query().Get("tld")
	if tld == "" {
		if cfg.DNS.TLD != "" {
			return cfg.DNS.TLD, nil
		}
		return "test", nil
	}
	if _, err := config.ValidateTLD(tld, cfg.DNS.Enabled); err != nil {
		return "", err
	}
	return tld, nil
}

// reapplyDNSForNewTLD rewrites the dnsmasq config for the full active TLD set
// and restarts lerd-dns (neither needs root), so the new ending is answered by
// dnsmasq immediately. It returns a user-facing warning when the platform
// resolver still needs a privileged, interactive step the daemon cannot perform
// (writing /etc/resolver/<tld> on macOS, which requires sudo). Returns "" when
// nothing more is needed.
func reapplyDNSForNewTLD(cfg *config.GlobalConfig, tld string) string {
	if cfg == nil || !cfg.DNS.Enabled {
		return ""
	}
	// Only the dnsmasq config changed (a new address= line), so a plain restart
	// of lerd-dns is enough — no daemon-reload (no unit files changed).
	if err := dns.WriteDnsmasqConfig(config.DnsmasqDir()); err != nil {
		fmt.Fprintf(os.Stderr, "lerd-ui: dnsmasq config for new TLD .%s: %v\n", tld, err)
	}
	if err := services.Mgr.Restart("lerd-dns"); err != nil {
		fmt.Fprintf(os.Stderr, "lerd-ui: restart lerd-dns: %v\n", err)
	}

	switch tld {
	case "local", "localhost":
		// lerd never writes an /etc/resolver entry for these — the OS owns them
		// (mDNS/Bonjour for .local, the loopback special-case for .localhost),
		// below the resolver layer. So there is no privileged step to run, and
		// whether they resolve depends on the system, not lerd.
		return "Note: ." + tld + " is resolved by your operating system (mDNS/Bonjour on macOS, " +
			"Avahi on Linux), not by lerd's DNS — so there's no extra step to run, and whether it " +
			"resolves depends on your machine. lerd's DNS now also answers ." + tld + " for tools " +
			"that query it directly. For a name that resolves reliably everywhere, prefer .test or a " +
			"custom ending like .lab."
	default:
		// A genuine custom TLD needs a system resolver entry. Try to apply it
		// automatically via the root-owned helper (installed once by `lerd
		// install`/`dns:apply`). If that succeeds, no action is needed; if the
		// helper isn't installed/granted yet, fall back to the manual command.
		if err := dns.AutoApplyResolver(); err == nil {
			return ""
		}
		return "Added on a new ending (." + tld + "). It isn't resolving yet — run `lerd dns:apply` " +
			"once from a terminal to enable ." + tld + ". (That one-time step also lets lerd set up " +
			"future endings from the dashboard automatically.)"
	}
}
