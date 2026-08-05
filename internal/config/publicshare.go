package config

import (
	"fmt"
	"strings"
)

// Public shares expose a site through the user's own reverse proxy: lerd runs a
// Host-rewriting proxy on a stable port, and the user points a wildcard subdomain
// they control at "<site>.<base>" → that port. The base and the derived hostnames
// live here; the proxy lifecycle lives in the cli package next to LAN share.

// NormalizePublicBase validates and normalizes the base domain public shares are
// served under, as "<site>.<base>". It is a domain the user controls with a
// wildcard pointed at this machine (example.com, dev.example.com); each site
// becomes a subdomain of it. A bare TLD is refused, since it would make the URL
// "<site>.com" and lerd serves local development, not hosting: that is enforced
// by requiring at least two labels. Empty stays empty (no base configured).
func NormalizePublicBase(s string) (string, error) {
	d := strings.Trim(strings.ToLower(strings.TrimSpace(s)), ".")
	if d == "" {
		return "", nil
	}
	if strings.ContainsAny(d, "/:@ \t") || strings.Contains(d, "*") {
		return "", fmt.Errorf("public base %q must be a bare domain, without a scheme, port, path or wildcard", s)
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("public base %q must be a domain like dev.example.com, not a bare TLD: sites are served as a subdomain of it", s)
	}
	for _, label := range labels {
		if !isPublicHostLabel(label) {
			return "", fmt.Errorf("public base %q is not a valid domain name", s)
		}
	}
	return d, nil
}

// PublicShareHost is the hostname a site is served on under the public base:
// "<site>.<base>". The site name is already a DNS-safe label and the base has
// been normalized, so the result is a valid subdomain.
func PublicShareHost(siteName, base string) string {
	if base == "" {
		return ""
	}
	return strings.ToLower(siteName) + "." + base
}

// PublicShareWorktreeHost is the hostname a worktree is served on under the
// public base. A worktree's own domain is "<branch>.<site>.<tld>", which is two
// labels under the site; flattened to one label under the base as
// "<site>-<branch>.<base>" so a single "*.<base>" wildcard covers it.
func PublicShareWorktreeHost(siteName, branchSlug, base string) string {
	if base == "" {
		return ""
	}
	return strings.ToLower(siteName) + "-" + strings.ToLower(branchSlug) + "." + base
}

// PublicShareURL is the address a public share answers on. The user's proxy is
// expected to terminate TLS, so it is shown over https.
func PublicShareURL(host string) string {
	if host == "" {
		return ""
	}
	return "https://" + host
}

// isPublicHostLabel reports whether s is usable as one label of a hostname.
func isPublicHostLabel(s string) bool {
	if s == "" || len(s) > 63 || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
