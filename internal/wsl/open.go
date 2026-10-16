package wsl

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// windowsPathFn translates a Linux path into the one Windows sees; a var so
// tests run without wslpath.
var windowsPathFn = func(p string) (string, error) {
	out, err := exec.Command("wslpath", "-w", p).Output()
	return strings.TrimSpace(string(out)), err
}

// openTarget is what explorer.exe is handed: a URL as is, a Linux path as its
// Windows form, since interop passes arguments through untranslated.
func openTarget(target string) string {
	if !strings.HasPrefix(target, "/") {
		return target
	}
	if p, err := windowsPathFn(target); err == nil && p != "" {
		return p
	}
	return target
}

// OpenCommand hands target to Windows: a URL opens in the default browser and
// a folder or file in its default app. explorer.exe exits 1 even when it
// opened the target, so callers Start it and ignore the exit status.
func OpenCommand(target string) *exec.Cmd {
	return exec.Command(windowsFile(`C:\Windows\explorer.exe`, "explorer.exe"), openTarget(target))
}

var openCommandFn = OpenCommand

// Open hands target to Windows and waits for explorer.exe, which exits once it
// has passed the target on. A short-lived lerd command must not exit first:
// WSL kills what a console-less session started, the tray's clicks included.
func Open(target string) error {
	err := openCommandFn(target).Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return nil // explorer.exe exits 1 even when it opened the target
	}
	return err
}

// parseWindowsPATH splits the output of `echo %PATH%` into its directories.
func parseWindowsPATH(out string) []string {
	var dirs []string
	for _, d := range strings.Split(strings.TrimSpace(out), ";") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// windowsPATHFn returns the Windows PATH as Linux directories. A login shell
// carries it, but a systemd service such as lerd-ui does not, so it is asked
// of cmd.exe once per process.
var windowsPATHFn = sync.OnceValue(func() []string {
	cmd := exec.Command(WindowsTool("cmd.exe"), "/c", "echo %PATH%")
	cmd.Dir = "/"
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, d := range parseWindowsPATH(string(out)) {
		if p, err := exec.Command("wslpath", "-u", d).Output(); err == nil {
			dirs = append(dirs, strings.TrimSpace(string(p)))
		}
	}
	return dirs
})

// LookPath finds a program on PATH and then on the Windows PATH, which is
// where a Windows editor's WSL launcher (Zed's or VS Code's bin dir) lives.
func LookPath(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range windowsPATHFn() {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// distroFromRoot reads the distro name out of the UNC path WSL gives "/".
func distroFromRoot(root string) string {
	for _, host := range []string{`\\wsl.localhost\`, `\\wsl$\`} {
		if strings.HasPrefix(root, host) {
			return strings.Trim(strings.TrimPrefix(root, host), `\`)
		}
	}
	return ""
}

// Distro names the distro lerd runs in. Shells get WSL_DISTRO_NAME, systemd
// services do not, so it falls back to the UNC path of the root.
func Distro() string {
	if d := os.Getenv("WSL_DISTRO_NAME"); d != "" {
		return d
	}
	root, _ := windowsPathFn("/")
	return distroFromRoot(root)
}

func terminalDirArgs(distro, dir string) []string {
	return []string{"-w", "0", "nt", "-p", distro, "--startingDirectory", openTarget(dir)}
}

// terminalScriptArgs runs script through `lerd wsl:term` in a new tab on the
// distro's own profile. wt.exe splits its command line on ';' and mangles
// quotes, so the script travels as base64, which has neither.
func terminalScriptArgs(distro, lerdBin, script string) []string {
	return []string{"-w", "0", "nt", "-p", distro, "wsl.exe", "-d", distro, "--exec", lerdBin, "wsl:term", base64.RawURLEncoding.EncodeToString([]byte(script))}
}

// DecodeTerminalScript reverses the encoding terminalScriptArgs applies.
func DecodeTerminalScript(arg string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(arg)
	return string(b), err
}

// TerminalDir is the Windows Terminal invocation that opens a tab on this
// distro at dir; ok is false when wt.exe is not installed.
func TerminalDir(dir string) (bin string, args []string, ok bool) {
	wt, err := LookPath("wt.exe")
	if err != nil {
		return "", nil, false
	}
	return wt, terminalDirArgs(Distro(), dir), true
}

// TerminalScript is the Windows Terminal invocation that runs script in a new
// tab through lerdBin; ok is false when wt.exe is not installed.
func TerminalScript(lerdBin, script string) (bin string, args []string, ok bool) {
	wt, err := LookPath("wt.exe")
	if err != nil {
		return "", nil, false
	}
	return wt, terminalScriptArgs(Distro(), lerdBin, script), true
}
