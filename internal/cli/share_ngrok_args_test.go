package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The argument is ngrok's own flags, so cobra must not try to parse it as
// lerd's: "--host-header=rewrite" reached it as an unknown flag before.
func TestShareNgrokArgsAcceptsNgroksOwnFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd := NewShareNgrokArgsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--host-header=rewrite", "--basic-auth", "me:secret pass"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("setting: %v", err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	// A value that carried a space was quoted on the command line, and has to
	// stay quoted or it comes back out of the config as two flags.
	got, err := splitNgrokArgs(cfg.Share.NgrokArgs)
	if err != nil {
		t.Fatalf("stored flags do not split back: %v", err)
	}
	if want := `--host-header=rewrite --basic-auth "me:secret pass"`; cfg.Share.NgrokArgs != want {
		t.Errorf("NgrokArgs = %q, want %q", cfg.Share.NgrokArgs, want)
	}
	if strings.Join(got, "|") != "--host-header=rewrite|--basic-auth|me:secret pass" {
		t.Errorf("round trip = %q", got)
	}
}

func TestShareNgrokArgsNoneClears(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareNgrokArgs(nil, []string{"--compression"}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	if err := runShareNgrokArgs(nil, []string{"none"}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.NgrokArgs != "" {
		t.Errorf("NgrokArgs = %q, want it cleared", cfg.Share.NgrokArgs)
	}
}

// A flag lerd owns is refused while it is still being typed, rather than at the
// next share, which would be a tunnel that never reports a URL.
func TestShareNgrokArgsRefusesTheFlagsLerdOwns(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := runShareNgrokArgs(nil, []string{"--log-format=json"}); err == nil {
		t.Fatal("a flag lerd sets itself should have been refused")
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.NgrokArgs != "" {
		t.Errorf("NgrokArgs = %q, want nothing stored", cfg.Share.NgrokArgs)
	}
}
