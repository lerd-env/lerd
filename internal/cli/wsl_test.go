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

func TestRemoveWindowsSideScript_DropsToastAppID(t *testing.T) {
	got := removeWindowsSideScript(`C:\Users\me\AppData\Local\lerd\bin`)
	if !strings.Contains(got, `Remove-Item -Path 'HKCU:\Software\Classes\AppUserModelId\sh.lerd.Lerd' -Recurse`) {
		t.Errorf("uninstall must drop the toast app id:\n%s", got)
	}
	if !strings.Contains(got, `$d = 'C:\Users\me\AppData\Local\lerd\bin'`) {
		t.Errorf("uninstall must still drop the bin dir from PATH:\n%s", got)
	}
}

func TestWindowsDriveWarning(t *testing.T) {
	got := windowsDriveWarning("/mnt/c/Users/me/app", "/home/me", "Ubuntu-24.04")
	for _, want := range []string{
		"/mnt/c/Users/me/app is on a Windows drive",
		"cp -a /mnt/c/Users/me/app /home/me/app",
		`\\wsl.localhost\Ubuntu-24.04\home\me\app`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning lacks %q:\n%s", want, got)
		}
	}
	// Without the distro name the Windows path is left out rather than wrong.
	if got := windowsDriveWarning("/mnt/c/app", "/home/me", ""); strings.Contains(got, "wsl.localhost") {
		t.Errorf("no distro, no UNC path:\n%s", got)
	}
}

// With the desktop app running the agent, an install from before it must not
// keep a Run entry starting a second one.
func TestAgentHandOverScript_DropsTheAgentRunEntry(t *testing.T) {
	got := agentHandOverScript()
	if !strings.Contains(got, `$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'`) ||
		!strings.Contains(got, `Remove-ItemProperty -Path $k -Name 'lerd'`) {
		t.Errorf("the hand-over must drop the agent's Run entry:\n%s", got)
	}
	// A fresh install has no entry to drop, and a bare Remove-ItemProperty
	// would fail the script for it, silenced or not.
	if !strings.Contains(got, `if (Get-ItemProperty -Path $k -Name 'lerd' -ErrorAction SilentlyContinue)`) {
		t.Errorf("the hand-over must only drop an entry that is there:\n%s", got)
	}
	if strings.Contains(got, "conhost") || strings.Contains(got, "Start-Process") {
		t.Errorf("the hand-over must not start the agent itself:\n%s", got)
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

// lerd install rewrites the tray unit and disables it on WSL, so off means not
// enabled and not running; a mask from an older wsl:setup counts too.
func TestTrayUnitOff(t *testing.T) {
	for _, c := range []struct {
		enabled, active string
		want            bool
	}{
		{"disabled", "inactive", true},
		{"masked", "inactive", true},
		{"", "inactive", true},
		{"enabled", "inactive", false},
		{"disabled", "active", false},
		{"disabled", "activating", false},
	} {
		if got := trayUnitOff(c.enabled, c.active); got != c.want {
			t.Errorf("trayUnitOff(%q, %q) = %v, want %v", c.enabled, c.active, got, c.want)
		}
	}
}

// Only the shell scripts lerd writes become .cmd files on Windows; the
// binaries and version stamps beside them in the bin dir do not.
func TestWindowsShimNames(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"php":                   "#!/bin/sh\nexec lerd php \"$@\"\n",
		"composer":              "#!/bin/sh\nexec lerd composer \"$@\"\n",
		"mkcert":                "\x7fELF binary",
		"composer.phar":         "<?php",
		"composer.phar.version": "2.8",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := windowsShimNames(dir)
	if strings.Join(got, ",") != "composer,php" {
		t.Errorf("windowsShimNames = %q, want [composer php]", got)
	}
}

// php.exe and its siblings are lerd.exe under another name: a .cmd would lose
// the current folder, since cmd.exe refuses a UNC one like \\wsl.localhost.
func TestWriteShimExes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("lerd.exe", "LERD")
	write("node.exe", "stale")
	write("php.cmd", "stale")
	write("lerd-wsl.conf", "distro=U")
	if err := writeShimExes(dir, []string{"php", "composer"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"php.exe", "composer.exe", "lerd.exe"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(b) != "LERD" {
			t.Errorf("%s = %q, %v; want lerd.exe's content", name, b, err)
		}
	}
	for _, name := range []string{"node.exe", "php.cmd"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s left behind", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "lerd-wsl.conf")); err != nil {
		t.Error("the config must survive")
	}
}

func TestMountRoot(t *testing.T) {
	for in, want := range map[string]string{"/mnt/c/": "/mnt", "/mnt/c": "/mnt", "/c/": "/", "": ""} {
		if got := mountRoot(in); got != want {
			t.Errorf("mountRoot(%q) = %q, want %q", in, got, want)
		}
	}
}
