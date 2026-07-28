package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestNormalizeBaseDomain_cleansUpWhatIsTyped(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"example.com", "example.com"},
		{"  Example.COM  ", "example.com"},
		{"example.com.", "example.com"},
		{".example.com", "example.com"},
		{"dev.example.co.uk", "dev.example.co.uk"},
	}
	for _, c := range cases {
		got, err := normalizeBaseDomain(c.in)
		if err != nil {
			t.Errorf("normalizeBaseDomain(%q) errored: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeBaseDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeBaseDomain_rejectsAnythingThatIsNotABareDomain(t *testing.T) {
	for _, in := range []string{
		"https://example.com",
		"example.com/app",
		"example.com:8080",
		"*.example.com",
		"example",
		"exa mple.com",
		"-example.com",
		"example-.com",
		"exa_mple.com",
	} {
		if got, err := normalizeBaseDomain(in); err == nil {
			t.Errorf("normalizeBaseDomain(%q) = %q, want an error", in, got)
		}
	}
}

func TestShareHostname_composesTheSiteDomainUnderTheBase(t *testing.T) {
	got, err := shareHostname("acme.test", "test", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "acme.example.com" {
		t.Errorf("hostname = %q, want acme.example.com", got)
	}
}

// The label follows the domain the user picked, not the folder the project sits
// in, so a scorediviner.test served out of a score-diviner directory is shared
// as scorediviner.
func TestShareHostLabel_followsTheDomainNotTheFolder(t *testing.T) {
	if got := shareHostLabel("scorediviner.test", "test"); got != "scorediviner" {
		t.Errorf("label = %q, want scorediviner", got)
	}
}

// A worktree's subdomain flattens into one label: a Cloudflare certificate
// covers one level of subdomain, so feat-acme.example.com works where
// feat.acme.example.com would not.
func TestShareHostLabel_flattensAWorktreeSubdomain(t *testing.T) {
	if got := shareHostLabel("feat-login.acme.test", "test"); got != "feat-login-acme" {
		t.Errorf("label = %q, want feat-login-acme", got)
	}
}

func TestShareHostLabel_honoursACustomTLD(t *testing.T) {
	if got := shareHostLabel("acme.localhost", "localhost"); got != "acme" {
		t.Errorf("label = %q, want acme", got)
	}
	// Unknown TLD: drop the last label rather than keeping the whole domain.
	if got := shareHostLabel("acme.dev", "test"); got != "acme" {
		t.Errorf("label = %q, want acme", got)
	}
}

func TestShareHostname_emptyBaseMeansNoHostname(t *testing.T) {
	got, err := shareHostname("acme.test", "test", "")
	if err != nil || got != "" {
		t.Errorf("shareHostname with no base = (%q, %v), want (\"\", nil)", got, err)
	}
}

func TestShareHostname_rejectsADomainThatCannotBeALabel(t *testing.T) {
	if _, err := shareHostname("my site.test", "test", "example.com"); err == nil {
		t.Error("expected an error for a domain that cannot be a hostname label")
	}
}

func TestResolveShareBaseDomain_prefersTheRequestedOverTheConfigured(t *testing.T) {
	got, err := resolveShareBaseDomain(&shareTool{mode: shareModeCloudflare}, "run.example.com", "saved.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "run.example.com" {
		t.Errorf("base = %q, want run.example.com", got)
	}
}

func TestResolveShareBaseDomain_fallsBackToTheConfigured(t *testing.T) {
	got, err := resolveShareBaseDomain(&shareTool{mode: shareModeCloudflare}, "", "saved.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "saved.example.com" {
		t.Errorf("base = %q, want saved.example.com", got)
	}
}

// The configured domain is a standing preference, not a per-run flag, so a tool
// that cannot serve a custom hostname just ignores it.
func TestResolveShareBaseDomain_otherToolsIgnoreTheConfiguredOne(t *testing.T) {
	got, err := resolveShareBaseDomain(&shareTool{mode: shareModeNgrok}, "", "saved.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("base = %q, want it dropped for ngrok", got)
	}
}

func TestResolveShareBaseDomain_rejectsARequestedOneOnAnotherTool(t *testing.T) {
	_, err := resolveShareBaseDomain(&shareTool{mode: shareModeNgrok}, "run.example.com", "")
	if err == nil || !strings.Contains(err.Error(), "Cloudflare") {
		t.Errorf("error = %v, want it to name Cloudflare Tunnel", err)
	}
}

func TestSetShareBaseDomain_remembersAndForgets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SetShareBaseDomain("Example.COM", true); err != nil {
		t.Fatalf("saving: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.BaseDomain != "example.com" || !cfg.Share.BaseDomainAnswered {
		t.Fatalf("stored (%q, %v), want (example.com, true)", cfg.Share.BaseDomain, cfg.Share.BaseDomainAnswered)
	}

	if err := SetShareBaseDomain("example.com", false); err != nil {
		t.Fatalf("forgetting: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.BaseDomain != "" || cfg.Share.BaseDomainAnswered {
		t.Errorf("stored (%q, %v), want it forgotten", cfg.Share.BaseDomain, cfg.Share.BaseDomainAnswered)
	}
}

// Skipping is an answer too: nothing is served under a domain, but the share
// menu has to stop asking.
func TestSetShareBaseDomain_rememberedSkipIsAnAnswer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SetShareBaseDomain("", true); err != nil {
		t.Fatalf("saving: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.BaseDomain != "" || !cfg.Share.BaseDomainAnswered {
		t.Errorf("stored (%q, %v), want (\"\", true)", cfg.Share.BaseDomain, cfg.Share.BaseDomainAnswered)
	}
}

func TestSetShareBaseDomain_rejectsAnInvalidDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SetShareBaseDomain("https://example.com/app", true); err == nil {
		t.Error("expected an error for a domain that is not bare")
	}
}

func TestShareTools_reportsTheBaseDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SetShareBaseDomain("example.com", true); err != nil {
		t.Fatalf("saving: %v", err)
	}

	info := ShareTools()
	if info.BaseDomain != "example.com" || !info.BaseDomainAnswered {
		t.Errorf("ShareTools reported (%q, %v), want (example.com, true)", info.BaseDomain, info.BaseDomainAnswered)
	}
}

func TestRunShareDomain_setShowClear(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := runShareDomain(nil, []string{"example.com"}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	out := captureStdout(t, func() {
		if err := runShareDomain(nil, nil); err != nil {
			t.Errorf("showing: %v", err)
		}
	})
	if !strings.Contains(out, "example.com") {
		t.Errorf("show output %q does not report the base domain", out)
	}
	if !strings.Contains(out, "lerd share:domain") {
		t.Errorf("show output %q does not say how to change it", out)
	}

	if err := runShareDomain(nil, []string{"none"}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.BaseDomain != "" || cfg.Share.BaseDomainAnswered {
		t.Errorf("stored (%q, %v) after none, want it cleared", cfg.Share.BaseDomain, cfg.Share.BaseDomainAnswered)
	}
}

func TestRunShareDomain_rejectsAnInvalidDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareDomain(nil, []string{"not a domain"}); err == nil {
		t.Error("expected an error for a domain that is not bare")
	}
}

// The dashboard has no terminal to run the browser login in, so it has to say
// what to run rather than block on a prompt nobody can answer.
func TestEnsureCloudflareTunnel_headlessRefusesToLogIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TUNNEL_ORIGIN_CERT", "")
	logFile := fakeCloudflared(t, "created", 0, "routed", 0)

	_, err := ensureCloudflareTunnel("mysite", "mysite.example.com", false)
	if err == nil || !strings.Contains(err.Error(), "cloudflared tunnel login") {
		t.Fatalf("error = %v, want it to name the login command", err)
	}
	if strings.Contains(readCalls(t, logFile), "tunnel login") {
		t.Error("a headless caller must not start an interactive login")
	}
}
