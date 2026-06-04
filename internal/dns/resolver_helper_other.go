//go:build !darwin

package dns

// InstallResolverHelper is a no-op off macOS: Linux routes every lerd ending
// through a single resolver file (the systemd-resolved drop-in or the
// NetworkManager dnsmasq conf), whose exact path is already covered by the
// passwordless sudoers grant — so no per-TLD helper is needed.
func InstallResolverHelper() error { return nil }

// AutoApplyResolver re-applies the resolver layer for the active set. On Linux
// ConfigureResolver rewrites the single drop-in via the already-granted
// exact-path sudo rules, so it runs without a password prompt from the daemon.
func AutoApplyResolver() error { return ConfigureResolver() }
