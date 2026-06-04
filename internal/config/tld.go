package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// tldShape is the syntactic rule for a single DNS label used as a TLD: 1–63
// chars, lowercase alphanumerics and hyphens, not starting or ending with a
// hyphen, and crucially no dots (multi-label suffixes like "dev.local" are not
// supported — lerd manages one label per resolver entry).
var tldShape = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// publicTLDs is an advisory (not exhaustive) set of real, ICANN-delegated
// gTLDs and the most common ccTLDs. A user TLD matching one of these is
// allowed but warned about, because lerd answering it locally shadows the real
// public domain — queries for genuine sites under that suffix break on this
// machine. This list only drives a warning, so it does not need to be complete.
var publicTLDs = map[string]bool{
	"com": true, "net": true, "org": true, "info": true, "biz": true,
	"dev": true, "app": true, "io": true, "co": true, "ai": true,
	"me": true, "tech": true, "site": true, "online": true, "store": true,
	"shop": true, "xyz": true, "cloud": true, "digital": true, "studio": true,
	"page": true, "zip": true, "mov": true, "uk": true, "us": true,
	"de": true, "fr": true, "ca": true, "au": true, "in": true,
}

// ExtractTLD returns the final dot-separated label of a domain, lowercased.
// "zotero_pro.test" → "test", "feature.app.local" → "local", "" → "".
func ExtractTLD(domain string) string {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if domain == "" {
		return ""
	}
	if i := strings.LastIndexByte(domain, '.'); i >= 0 {
		return domain[i+1:]
	}
	return domain
}

// ActiveTLDs returns the sorted, de-duplicated set of TLDs lerd's resolver
// layer must answer for: the suffix of every domain across every non-ignored
// registered site, plus the global default TLD (so a fresh `lerd link` works
// even before any site exists). This set — not a single DNS.TLD — drives the
// dnsmasq address= lines, the macOS /etc/resolver/<tld> files, and the Linux
// resolver routing domains once multi-TLD is wired through.
func ActiveTLDs() []string {
	set := map[string]bool{}

	if cfg, err := LoadGlobal(); err == nil && cfg != nil {
		if t := ExtractTLD(cfg.DNS.TLD); t != "" {
			set[t] = true
		}
	}

	if reg, err := LoadSites(); err == nil && reg != nil {
		for _, s := range reg.Sites {
			if s.Ignored {
				continue
			}
			for _, d := range s.Domains {
				if t := ExtractTLD(d); t != "" {
					set[t] = true
				}
			}
		}
	}

	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ValidateTLD checks a user-supplied TLD label for use as a lerd site suffix.
// It returns a non-nil error when the label must be rejected, and otherwise a
// (possibly empty) advisory warning the caller should surface but not treat as
// fatal — matching the "auto-accept any reasonable TLD, but be honest about
// caveats" policy.
//
// dnsEnabled distinguishes the two modes: with lerd-managed DNS on, "localhost"
// is rejected because the OS stub resolver pins *.localhost to loopback before
// dnsmasq is ever consulted (it can never be served, and it collides with the
// DNS-disabled sentinel). With DNS off, "localhost" is the expected value.
func ValidateTLD(tld string, dnsEnabled bool) (warning string, err error) {
	tld = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(tld, ".")))
	if tld == "" {
		return "", fmt.Errorf("TLD cannot be empty")
	}
	if strings.Contains(tld, ".") {
		return "", fmt.Errorf("TLD %q must be a single label (no dots) — multi-label suffixes are not supported", tld)
	}
	if !tldShape.MatchString(tld) {
		return "", fmt.Errorf("TLD %q is not a valid DNS label (use lowercase letters, digits and hyphens, 1–63 chars)", tld)
	}

	switch tld {
	case "localhost":
		if dnsEnabled {
			return "", fmt.Errorf("TLD %q cannot be used with lerd-managed DNS: the OS resolves *.localhost to loopback before dnsmasq, so it can't be served and collides with the DNS-disabled mode", tld)
		}
		return "", nil
	case "local":
		return ".local is reserved for mDNS/Bonjour: on macOS it resolves only the bare host via Bonjour (no subdomains, occasional delays, browsers may need Local Network permission); on Linux it requires disabling Avahi/nss-mdns and lerd will not do that automatically. Prefer .test for anything you rely on.", nil
	}

	if publicTLDs[tld] {
		return fmt.Sprintf(".%s is a real public domain suffix: answering it locally will shadow genuine %s sites on this machine. Prefer .test or a private label like .lab.", tld, tld), nil
	}

	return "", nil
}
