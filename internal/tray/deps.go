package tray

import (
	"bufio"
	"bytes"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// libSearchDirs are the standard locations checked when ldconfig's cache is
// unavailable (some minimal and immutable images ship no ldconfig binary).
var libSearchDirs = []string{"/lib64", "/usr/lib64", "/lib", "/usr/lib"}

// HelperPath returns the lerd-tray binary installed alongside lerd, or "" when
// the running executable can't be located.
func HelperPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "lerd-tray")
}

// Unavailable says why the tray helper cannot run here, or "" when it can: no
// helper beside this lerd (a local build installed on its own), or a shared
// library the host lacks.
func Unavailable(helper string) string {
	if _, err := os.Stat(helper); err != nil {
		return "no lerd-tray helper beside this lerd binary"
	}
	if missing := MissingLibs(helper); len(missing) > 0 {
		return "this host has no " + strings.Join(missing, ", ")
	}
	return ""
}

// MissingLibs returns the shared libraries the tray helper links that this host
// cannot resolve. lerd-tray needs libayatana-appindicator, which an immutable
// image cannot simply install: without the library the helper exits 127 on
// every start, so the unit is left restart-looping and the whole systemd user
// session reports "degraded". Detection is conservative — anything it cannot
// determine (unreadable binary, no ELF, no library index) reports nothing
// missing, so a working tray is never disabled by a failed probe.
func MissingLibs(helper string) []string {
	needed, err := neededLibs(helper)
	if err != nil || len(needed) == 0 {
		return nil
	}
	index := hostLibs()
	if len(index) == 0 {
		return nil
	}
	return missingFrom(needed, index)
}

// missingFrom returns the needed libraries absent from the host's library index.
func missingFrom(needed []string, index map[string]struct{}) []string {
	var missing []string
	for _, lib := range needed {
		if _, found := index[lib]; !found {
			missing = append(missing, lib)
		}
	}
	return missing
}

// neededLibs reads the DT_NEEDED entries of an ELF binary.
func neededLibs(path string) ([]string, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ImportedLibraries()
}

// hostLibs indexes every shared library the dynamic linker can resolve, from
// ldconfig's cache when it exists and from the standard directories otherwise.
func hostLibs() map[string]struct{} {
	index := map[string]struct{}{}
	if out, err := exec.Command("ldconfig", "-p").Output(); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			// "\tlibfoo.so.1 (libc6,x86-64) => /usr/lib64/libfoo.so.1"
			if name, _, found := strings.Cut(strings.TrimSpace(scanner.Text()), " "); found {
				index[name] = struct{}{}
			}
		}
	}
	for _, dir := range libSearchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			index[filepath.Base(e.Name())] = struct{}{}
		}
	}
	return index
}
