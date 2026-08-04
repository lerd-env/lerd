package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A bare token predates the provider argument, so it has to keep meaning ngrok.
func TestRunShareToken_bareTokenStaysNgrok(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareToken(nil, []string{"2abcXYZ"}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.NgrokToken != "2abcXYZ" {
		t.Errorf("NgrokToken = %q, want the bare token stored for ngrok", cfg.Share.NgrokToken)
	}
	if cfg.Share.PinggyToken != "" {
		t.Errorf("PinggyToken = %q, want it untouched", cfg.Share.PinggyToken)
	}
}

func TestRunShareToken_pinggySetAndClear(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareToken(nil, []string{"pinggy", "tok123"}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.PinggyToken != "tok123" {
		t.Errorf("PinggyToken = %q, want tok123", cfg.Share.PinggyToken)
	}
	if cfg.Share.NgrokToken != "" {
		t.Errorf("NgrokToken = %q, want it untouched", cfg.Share.NgrokToken)
	}

	if err := runShareToken(nil, []string{"pinggy", "none"}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if cfg.Share.PinggyToken != "" {
		t.Errorf("PinggyToken = %q after none, want it cleared", cfg.Share.PinggyToken)
	}
}

// The explicit provider spelling has to work for ngrok too, so nobody has to
// remember that the bare form implies it.
func TestRunShareToken_explicitNgrokProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareToken(nil, []string{"ngrok", "2abcXYZ"}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.NgrokToken != "2abcXYZ" {
		t.Errorf("NgrokToken = %q, want 2abcXYZ", cfg.Share.NgrokToken)
	}
}

// The status report covers both providers, and never prints a token back.
func TestRunShareToken_showReportsBothProviders(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareToken(nil, []string{"pinggy", "tok123"}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	out := captureStdout(t, func() {
		if err := runShareToken(nil, nil); err != nil {
			t.Errorf("showing: %v", err)
		}
	})
	for _, want := range []string{"ngrok", "pinggy"} {
		if !strings.Contains(strings.ToLower(out), want) {
			t.Errorf("show output %q does not mention %s", out, want)
		}
	}
	if strings.Contains(out, "tok123") {
		t.Errorf("show output %q prints the stored token back", out)
	}
}
