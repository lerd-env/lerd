package wsl

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func stubStartMenu(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	orig := startMenuFn
	startMenuFn = func() string { return root }
	t.Cleanup(func() { startMenuFn = orig })
	return root
}

// writeLnk writes a stand-in shortcut whose arguments are stored as UTF-16LE,
// the way a .lnk keeps them.
func writeLnk(t *testing.T, path, args string) {
	t.Helper()
	var b []byte
	for _, u := range utf16.Encode([]rune(args)) {
		b = append(b, byte(u), byte(u>>8))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append([]byte("L\x00\x00\x00"), b...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPWAShortcut_findsTheBrowserAppShortcut(t *testing.T) {
	root := stubStartMenu(t)
	want := filepath.Join(root, "Aplicaciones de Vivaldi", "Lerd.lnk")
	writeLnk(t, want, " --profile-directory=Default --app-id=fpnfgjllafoobfhcdpnnhnkahdhldhoo")

	got, ok := PWAShortcut()

	if !ok || got != want {
		t.Errorf("got %q, %v; want %q", got, ok, want)
	}
}

func TestPWAShortcut_ignoresOtherLerdShortcuts(t *testing.T) {
	root := stubStartMenu(t)
	writeLnk(t, filepath.Join(root, "Lerd", "Lerd.lnk"), "--agent")

	if got, ok := PWAShortcut(); ok {
		t.Errorf("a shortcut without --app-id is not the dashboard app: %q", got)
	}
}

func TestPWAShortcut_noStartMenu(t *testing.T) {
	orig := startMenuFn
	startMenuFn = func() string { return "" }
	t.Cleanup(func() { startMenuFn = orig })

	if _, ok := PWAShortcut(); ok {
		t.Error("no Start Menu, no app")
	}
}
