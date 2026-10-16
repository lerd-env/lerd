package wsl

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func stubWindowsPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := windowsPathFn
	windowsPathFn = fn
	t.Cleanup(func() { windowsPathFn = orig })
}

func stubOpenCommand(t *testing.T, script string) {
	t.Helper()
	orig := openCommandFn
	openCommandFn = func(string) *exec.Cmd { return exec.Command("sh", "-c", script) }
	t.Cleanup(func() { openCommandFn = orig })
}

func TestOpen_waitsForTheOpenerToHandOff(t *testing.T) {
	done := filepath.Join(t.TempDir(), "handed-off")
	stubOpenCommand(t, "sleep 0.2; touch "+done)

	if err := Open("http://lerd.localhost"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(done); err != nil {
		t.Error("Open returned before the opener finished")
	}
}

func TestOpen_ignoresExplorersExitStatus(t *testing.T) {
	stubOpenCommand(t, "exit 1")
	if err := Open("http://lerd.localhost"); err != nil {
		t.Errorf("explorer.exe exits 1 on success, got %v", err)
	}
}

func stubWindowsPATH(t *testing.T, dirs []string) {
	t.Helper()
	orig := windowsPATHFn
	windowsPATHFn = func() []string { return dirs }
	t.Cleanup(func() { windowsPATHFn = orig })
}

func TestOpenTarget_translatesLinuxPaths(t *testing.T) {
	stubWindowsPath(t, func(p string) (string, error) {
		return `\\wsl.localhost\Ubuntu\` + strings.ReplaceAll(strings.TrimPrefix(p, "/"), "/", `\`), nil
	})
	if got := openTarget("/home/me/app"); got != `\\wsl.localhost\Ubuntu\home\me\app` {
		t.Errorf("openTarget(path) = %q", got)
	}
	if got := openTarget("https://app.test/"); got != "https://app.test/" {
		t.Errorf("openTarget(url) = %q, want it untouched", got)
	}
}

func TestOpenTarget_keepsThePathWhenWslpathFails(t *testing.T) {
	stubWindowsPath(t, func(string) (string, error) { return "", os.ErrNotExist })
	if got := openTarget("/home/me/app"); got != "/home/me/app" {
		t.Errorf("openTarget = %q, want the original path", got)
	}
}

func TestParseWindowsPATH_splitsAndSkipsEmpties(t *testing.T) {
	got := parseWindowsPATH("C:\\Windows\\system32;;C:\\Program Files\\Zed\\bin;\r\n")
	want := []string{`C:\Windows\system32`, `C:\Program Files\Zed\bin`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWindowsPATH = %q, want %q", got, want)
	}
}

func TestLookPath_fallsBackToTheWindowsPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	bin := filepath.Join(dir, "zed")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubWindowsPATH(t, []string{filepath.Join(dir, "missing"), dir})
	got, err := LookPath("zed")
	if err != nil || got != bin {
		t.Errorf("LookPath = %q, %v; want %q", got, err, bin)
	}
	if _, err := LookPath("code"); err == nil {
		t.Error("LookPath found a program that is on neither PATH")
	}
}

func TestDistroFromRoot_readsTheUNCRoot(t *testing.T) {
	for in, want := range map[string]string{
		`\\wsl.localhost\Ubuntu-24.04\`: "Ubuntu-24.04",
		`\\wsl$\Debian\`:                "Debian",
		`C:\`:                           "",
	} {
		if got := distroFromRoot(in); got != want {
			t.Errorf("distroFromRoot(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTerminalArgs_opensATabOnTheDistroAtDir(t *testing.T) {
	stubWindowsPath(t, func(p string) (string, error) { return `\\wsl.localhost\Ubuntu` + strings.ReplaceAll(p, "/", `\`), nil })
	got := terminalDirArgs("Ubuntu", "/home/me/app")
	want := []string{"-w", "0", "nt", "-p", "Ubuntu", "--startingDirectory", `\\wsl.localhost\Ubuntu\home\me\app`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("terminalDirArgs = %q, want %q", got, want)
	}
}

// wt.exe splits its command line on ';' and mangles quotes, so the script
// rides as base64 and comes back intact.
func TestTerminalScriptArgs_carriesTheScriptAsBase64(t *testing.T) {
	script := `lerd update; echo; read -rp "Press Enter" x`
	got := terminalScriptArgs("Ubuntu", "/home/me/.local/bin/lerd", script)
	want := []string{"-w", "0", "nt", "-p", "Ubuntu", "wsl.exe", "-d", "Ubuntu", "--exec", "/home/me/.local/bin/lerd", "wsl:term"}
	if !reflect.DeepEqual(got[:len(want)], want) || len(got) != len(want)+1 {
		t.Fatalf("terminalScriptArgs = %q", got)
	}
	if strings.ContainsAny(got[len(want)], "; \"'") {
		t.Errorf("encoded script %q carries characters wt.exe splits or mangles", got[len(want)])
	}
	if back, err := DecodeTerminalScript(got[len(want)]); err != nil || back != script {
		t.Errorf("DecodeTerminalScript = %q, %v; want %q", back, err, script)
	}
}
