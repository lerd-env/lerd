package config

import (
	"strings"
	"testing"
)

func TestNormalizePublicBase(t *testing.T) {
	ok := map[string]string{
		"example.com":       "example.com",
		"dev.example.com":   "dev.example.com",
		"  Dev.Example.COM": "dev.example.com",
		".vpn.example.org.": "vpn.example.org",
	}
	for in, want := range ok {
		got, err := NormalizePublicBase(in)
		if err != nil || got != want {
			t.Errorf("NormalizePublicBase(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := NormalizePublicBase("  "); err != nil || got != "" {
		t.Errorf("blank should stay empty, got %q, %v", got, err)
	}
	for _, bad := range []string{"com", "localhost", "*.example.com", "https://example.com", "example.com:8080", "a..b", "-x.example.com"} {
		if _, err := NormalizePublicBase(bad); err == nil {
			t.Errorf("NormalizePublicBase(%q) accepted, want rejected", bad)
		}
	}
	if _, err := NormalizePublicBase("com"); err == nil || !strings.Contains(err.Error(), "TLD") {
		t.Errorf("bare TLD should mention TLD, got %v", err)
	}
}

func TestPublicShareHosts(t *testing.T) {
	if got := PublicShareHost("MyApp", "dev.example.com"); got != "myapp.dev.example.com" {
		t.Errorf("host = %q", got)
	}
	if got := PublicShareWorktreeHost("myapp", "feature", "dev.example.com"); got != "myapp-feature.dev.example.com" {
		t.Errorf("worktree host = %q, want flat myapp-feature.dev.example.com", got)
	}
	if PublicShareHost("myapp", "") != "" {
		t.Error("no base should yield empty host")
	}
	if got := PublicShareURL("myapp.dev.example.com"); got != "https://myapp.dev.example.com" {
		t.Errorf("url = %q", got)
	}
}
