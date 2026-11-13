package desktopnotify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// An icon cached by an older lerd must give way to the bundled one, or a new
// logo never reaches notifications on a machine that already had lerd.
func TestIconPathReplacesStaleIcon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(base, "lerd", "notify-icon.png")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old logo"), 0o644); err != nil {
		t.Fatal(err)
	}
	iconOnce, iconFile = sync.Once{}, ""
	t.Cleanup(func() { iconOnce, iconFile = sync.Once{}, "" })

	if got := IconPath(); got != stale {
		t.Fatalf("IconPath() = %q, want %q", got, stale)
	}
	if got, _ := os.ReadFile(stale); !bytes.Equal(got, iconPNG) {
		t.Error("a stale cached icon must be replaced by the bundled one")
	}
}

// Windows keeps its own copy of the icon; it must be refreshed from the distro,
// not only created when missing.
func TestToastScriptRefreshesIcon(t *testing.T) {
	s := toastScript(Request{Summary: "s"}, `\\wsl.localhost\Ubuntu\icon.png`)
	if strings.Contains(s, `-not (Test-Path -LiteralPath $icon)`) {
		t.Errorf("the Windows icon copy must not depend on it being missing:\n%s", s)
	}
}
