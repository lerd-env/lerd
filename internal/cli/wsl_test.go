package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lerd ships its own mkcert under BinDir and never puts it on PATH, so the CA
// has to be found through that copy or wsl:setup skips the Windows trust step
// on every normal install.
func TestMkcertRootCAPEM_UsesLerdsOwnMkcert(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("PATH", "")

	caroot := filepath.Join(data, "caroot")
	if err := os.MkdirAll(caroot, 0o755); err != nil {
		t.Fatal(err)
	}
	pem := filepath.Join(caroot, "rootCA.pem")
	if err := os.WriteFile(pem, []byte("pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(data, "lerd", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho " + caroot + "\n"
	if err := os.WriteFile(filepath.Join(bin, "mkcert"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := mkcertRootCAPEM()
	if !ok || got != pem {
		t.Fatalf("mkcertRootCAPEM() = %q, %v; want %q, true", got, ok, pem)
	}
}

func TestMkcertRootCAPEM_NoCAYet(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", "")
	if got, ok := mkcertRootCAPEM(); ok {
		t.Fatalf("expected no CA without mkcert, got %q", got)
	}
}

func TestEncodePowerShell_UTF16LEBase64(t *testing.T) {
	// powershell -EncodedCommand takes base64 of UTF-16LE; "ab" is 61 00 62 00.
	if got := encodePowerShell("ab"); got != "YQBiAA==" {
		t.Errorf("got %q, want YQBiAA==", got)
	}
}

func TestAgentRunValue_Headless(t *testing.T) {
	got := agentRunValue(`C:\Users\me\AppData\Local\lerd\bin`)
	want := `conhost.exe --headless "C:\Users\me\AppData\Local\lerd\bin\lerd.exe" --agent`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplaceRunningExe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lerd.exe")

	if err := replaceRunningExe(path, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".old"); err == nil {
		t.Error("a first install has nothing to move aside")
	}

	// A newer build moves the running one aside rather than overwriting it.
	if err := replaceRunningExe(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "v2" {
		t.Errorf("new exe = %q, want v2", b)
	}
	if b, _ := os.ReadFile(path + ".old"); string(b) != "v1" {
		t.Errorf("old exe = %q, want v1", b)
	}

	// The same build again touches nothing.
	os.Remove(path + ".old")
	if err := replaceRunningExe(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".old"); err == nil {
		t.Error("an unchanged exe should not be moved aside")
	}
}

func TestPinFoldersScript_QuotesPaths(t *testing.T) {
	got := pinFoldersScript([]string{`\\wsl.localhost\Ubuntu\home\o'brien\Lerd`})
	if !strings.Contains(got, `@('\\wsl.localhost\Ubuntu\home\o''brien\Lerd')`) {
		t.Errorf("path not quoted for PowerShell:\n%s", got)
	}
	if !strings.Contains(got, "pintohome") || !strings.Contains(got, quickAccess) {
		t.Errorf("script does not pin to Quick access:\n%s", got)
	}
}
