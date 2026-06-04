package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/spf13/cobra"
)

// resolveDomainTLD returns the single TLD for a `lerd domain add/remove`
// operation: the --tld flag (validated) or the global default when unset.
func resolveDomainTLD(cmd *cobra.Command, cfg *config.GlobalConfig) (string, error) {
	flag, _ := cmd.Flags().GetString("tld")
	tlds, err := parseTLDFlag(flag, cfg)
	if err != nil {
		return "", err
	}
	return tlds[0], nil
}

// tldInActiveSet reports whether tld is already answered by the resolver (i.e.
// some registered site already uses it).
func tldInActiveSet(tld string) bool {
	for _, t := range config.ActiveTLDs() {
		if t == tld {
			return true
		}
	}
	return false
}

// introducedTLDs returns the TLDs among domains that were not already active
// (per the pre-operation snapshot), so callers know whether the resolver layer
// needs re-applying.
func introducedTLDs(preActive map[string]bool, domains []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range domains {
		t := config.ExtractTLD(d)
		if t == "" || preActive[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// parseTLDFlag splits a --tld value ("test,local" or "test local") into a
// validated, de-duplicated, order-preserving list. Each label is validated
// against config.ValidateTLD; a hard error aborts, a warning is printed but
// kept. An empty flag yields the global default TLD.
func parseTLDFlag(value string, cfg *config.GlobalConfig) ([]string, error) {
	def := cfg.DNS.TLD
	if def == "" {
		def = "test"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{def}, nil
	}

	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' }) {
		tld := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "."))
		if tld == "" || seen[tld] {
			continue
		}
		warn, err := config.ValidateTLD(tld, cfg.DNS.Enabled)
		if err != nil {
			return nil, err
		}
		if warn != "" {
			fmt.Printf("Note (.%s): %s\n", tld, warn)
		}
		seen[tld] = true
		out = append(out, tld)
	}
	if len(out) == 0 {
		return []string{def}, nil
	}
	return out, nil
}

// knownTLDs returns the set of TLD labels lerd currently recognises: the active
// set (suffixes of every registered site) plus the global default and any extra
// passed in (e.g. a --tld flag's values). Used to tell whether a .lerd.yaml
// domains entry is already a full domain (ends in a known TLD) versus a bare
// label that needs the default TLD appended.
func knownTLDs(cfg *config.GlobalConfig, extra ...string) map[string]bool {
	set := map[string]bool{}
	for _, t := range config.ActiveTLDs() {
		set[t] = true
	}
	if cfg != nil && cfg.DNS.TLD != "" {
		set[cfg.DNS.TLD] = true
	}
	for _, t := range extra {
		if t != "" {
			set[t] = true
		}
	}
	return set
}

// NewDNSApplyCmd returns the `dns:apply` command: re-applies the resolver layer
// for every ending your sites use. Run it from a terminal after adding a site on
// a new custom ending from the dashboard — the background service can't create
// the privileged /etc/resolver entry, but this command can (prompting for sudo).
func NewDNSApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dns:apply",
		Short: "Re-apply DNS resolution for every ending your sites use (may prompt for sudo)",
		Long: "Regenerates the dnsmasq config and the per-ending system resolver entries " +
			"(/etc/resolver/<tld> on macOS) for every TLD across your registered sites. " +
			"Run this once from a terminal after adding a site on a new custom ending from " +
			"the dashboard, since the background service can't perform the privileged write itself.",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if !cfg.DNS.Enabled {
				fmt.Println("lerd-managed DNS is disabled — nothing to apply (sites use the system resolver).")
				return nil
			}
			tlds := config.ActiveTLDs()
			labels := make([]string, len(tlds))
			for i, t := range tlds {
				labels[i] = "." + t
			}
			fmt.Printf("Applying DNS resolution for: %s\n", strings.Join(labels, " "))
			if err := EnsureTLDResolution(cfg); err != nil {
				return err
			}
			fmt.Println("Done. Your sites' endings now resolve through lerd-dns.")
			return nil
		},
	}
}

// EnsureTLDResolution re-applies the resolver layer after the active TLD set may
// have grown (a site linked or aliased on a new TLD): it rewrites lerd.conf for
// the full active set, restarts lerd-dns, (re)grants the macOS resolver sudoers
// for any new TLD, and refreshes the platform resolver entries. No-op when
// lerd-managed DNS is disabled. Best-effort — the error is for the caller to
// warn on, not fatal to the surrounding operation.
func EnsureTLDResolution(cfg *config.GlobalConfig) error {
	if cfg == nil || !cfg.DNS.Enabled {
		return nil
	}
	if err := dns.WriteDnsmasqConfig(config.DnsmasqDir()); err != nil {
		return fmt.Errorf("rewriting dnsmasq config: %w", err)
	}
	if err := reloadAndRestartUnit("lerd-dns"); err != nil {
		return fmt.Errorf("restarting lerd-dns: %w", err)
	}
	// Grant passwordless resolver writes for any newly active TLD (macOS) so the
	// repair watcher can keep it up to date, and lay down the root-owned resolver
	// helper so the dashboard can enable future endings on its own. Both run an
	// interactive sudo, so this only happens from a terminal context (install /
	// dns:apply / CLI domain add), never the background daemon.
	_ = dns.InstallSudoers()
	_ = dns.InstallResolverHelper()
	if err := dns.ConfigureResolver(); err != nil {
		return fmt.Errorf("configuring resolver: %w", err)
	}
	return nil
}
