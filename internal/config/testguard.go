package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// realStateDirs are the lerd dirs of whoever is running the process, resolved once
// at start. XDG_DATA_HOME and XDG_CONFIG_HOME decide them and a test moves those,
// so resolving later can't tell a test's temp dir from the developer's own. The
// unit dirs are here too: the bug that started this wrote a real systemd unit.
var realStateDirs = []string{DataDir(), ConfigDir(), SystemdUserDir(), QuadletDir()}

// underTest reports whether this process is a test binary, read from the command
// line rather than testing.Testing() so the std testing package stays out of the
// shipped lerd binary.
var underTest = func() bool {
	if strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/") {
		return true
	}
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "-test.") {
			return true
		}
	}
	return false
}()

// GuardRealWrite is guardRealWrite for writers in other packages.
func GuardRealWrite(path string) { guardRealWrite(path) }

// guardRealWrite stops a test writing the state of the developer running it. A
// test that never isolated XDG_DATA_HOME has emptied a real sites.yaml, so make
// that a loud failure at the write rather than silent data loss.
func guardRealWrite(path string) {
	if !underTest {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	abs = resolveLinks(abs)
	for _, real := range realStateDirs {
		if real == "" {
			continue
		}
		real = resolveLinks(real)
		if abs == real || strings.HasPrefix(abs, real+string(os.PathSeparator)) {
			panic(fmt.Sprintf("test wrote to the real lerd state at %s: isolate it with "+
				`t.Setenv("XDG_DATA_HOME", t.TempDir())`+" and "+`t.Setenv("XDG_CONFIG_HOME", t.TempDir())`, abs))
		}
	}
}

// resolveLinks canonicalises what exists of path, so a symlinked state dir can't
// slip past the prefix comparison.
func resolveLinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
