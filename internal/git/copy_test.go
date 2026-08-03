package git

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCopyTreeNative_RegularFilesAndDirs(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	if err := os.MkdirAll(filepath.Join(src, "sub/nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub/nested/inner.txt"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyTreeNative(src, dst); err != nil {
		t.Fatalf("copyTreeNative: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("top.txt: got %q err %v, want %q nil", got, err, "hello")
	}

	got, err = os.ReadFile(filepath.Join(dst, "sub/nested/inner.txt"))
	if err != nil || string(got) != "world" {
		t.Fatalf("inner.txt: got %q err %v, want %q nil", got, err, "world")
	}

	info, err := os.Stat(filepath.Join(dst, "sub/nested/inner.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("inner.txt perm: got %o want 600", perm)
	}
}

func TestCopyTreeNative_PreservesInnerSymlinks(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	if err := os.WriteFile(filepath.Join(src, "real.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(src, "link.txt")); err != nil {
		t.Fatal(err)
	}

	if err := copyTreeNative(src, dst); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("link.txt is not a symlink in dst")
	}
	target, err := os.Readlink(filepath.Join(dst, "link.txt"))
	if err != nil || target != "real.txt" {
		t.Errorf("link target: got %q err %v, want %q", target, err, "real.txt")
	}
}

func TestJSPackageManager_DetectsLockfile(t *testing.T) {
	cases := []struct {
		name     string
		lockfile string
		wantBin  string
		wantArg0 string
	}{
		{"pnpm", "pnpm-lock.yaml", "pnpm", "install"},
		{"yarn", "yarn.lock", "yarn", "install"},
		{"bun-binary", "bun.lockb", "bun", "install"},
		{"bun-text", "bun.lock", "bun", "install"},
		{"npm-lockfile", "package-lock.json", "npm", "ci"},
		{"npm-shrinkwrap", "npm-shrinkwrap.json", "npm", "ci"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, c.lockfile), []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}
			gotBin, gotArgs := jsPackageManager(dir)
			if gotBin != c.wantBin {
				t.Errorf("binary: got %q want %q", gotBin, c.wantBin)
			}
			if len(gotArgs) == 0 || gotArgs[0] != c.wantArg0 {
				t.Errorf("first arg: got %v want %q", gotArgs, c.wantArg0)
			}
		})
	}
}

func TestJSPackageManager_DefaultsToNpmInstallWithoutLockfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, args := jsPackageManager(dir)
	if bin != "npm" {
		t.Errorf("binary: got %q want npm", bin)
	}
	if len(args) == 0 || args[0] != "install" {
		t.Errorf("first arg: got %v want install", args)
	}
}

func TestJSPackageManager_PnpmBeatsOtherLockfiles(t *testing.T) {
	// Defensive: if a repo somehow has both pnpm-lock.yaml and
	// package-lock.json, the pnpm lockfile is the modern one — prefer it.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, lf := range []string{"pnpm-lock.yaml", "package-lock.json"} {
		if err := os.WriteFile(filepath.Join(dir, lf), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin, _ := jsPackageManager(dir)
	if bin != "pnpm" {
		t.Errorf("expected pnpm to win over npm lockfile, got %q", bin)
	}
}

// NeedsInstall is the OR of composer+JS install-needed checks. Sanity:
// composer.json without vendor → true; both fully installed → false.
func TestNeedsInstall(t *testing.T) {
	dir := t.TempDir()
	if NeedsInstall(dir) {
		t.Fatal("empty dir should not need install")
	}
	touch(t, filepath.Join(dir, "composer.json"))
	if !NeedsInstall(dir) {
		t.Fatal("composer.json with no vendor must need install")
	}
	touchAt(t, filepath.Join(dir, "composer.lock"), time.Now().Add(-time.Hour))
	touch(t, filepath.Join(dir, "vendor", "composer", "installed.json"))
	if NeedsInstall(dir) {
		t.Fatal("composer.json + installed.json newer than lock must not need install")
	}
}

func TestComposerNeedsInstall(t *testing.T) {
	cases := []struct {
		name string
		set  func(dir string)
		want bool
	}{
		{
			name: "no composer.json",
			set:  func(string) {},
			want: false,
		},
		{
			name: "lock newer than installed.json",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "composer.json"))
				touch(t, filepath.Join(dir, "composer.lock"))
				touchAt(t, filepath.Join(dir, "vendor", "composer", "installed.json"), time.Now().Add(-time.Hour))
			},
			want: true,
		},
		{
			name: "installed.json newer than lock",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "composer.json"))
				touchAt(t, filepath.Join(dir, "composer.lock"), time.Now().Add(-time.Hour))
				touch(t, filepath.Join(dir, "vendor", "composer", "installed.json"))
			},
			want: false,
		},
		{
			name: "no installed.json marker",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "composer.json"))
				touch(t, filepath.Join(dir, "composer.lock"))
			},
			want: true,
		},
		{
			name: "no lockfile but installed.json exists",
			set: func(dir string) {
				touchAt(t, filepath.Join(dir, "composer.json"), time.Now().Add(-time.Hour))
				touch(t, filepath.Join(dir, "vendor", "composer", "installed.json"))
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			c.set(dir)
			if got := composerNeedsInstall(dir); got != c.want {
				t.Errorf("composerNeedsInstall=%v want %v", got, c.want)
			}
		})
	}
}

func TestJSNeedsInstall(t *testing.T) {
	cases := []struct {
		name string
		set  func(dir string)
		want bool
	}{
		{
			name: "no package.json",
			set:  func(string) {},
			want: false,
		},
		{
			name: "npm: lock newer than node_modules marker",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "package.json"))
				touch(t, filepath.Join(dir, "package-lock.json"))
				touchAt(t, filepath.Join(dir, "node_modules", ".package-lock.json"), time.Now().Add(-time.Hour))
			},
			want: true,
		},
		{
			name: "npm: marker newer than lock",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "package.json"))
				touchAt(t, filepath.Join(dir, "package-lock.json"), time.Now().Add(-time.Hour))
				touch(t, filepath.Join(dir, "node_modules", ".package-lock.json"))
			},
			want: false,
		},
		{
			name: "npm: no node_modules at all",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "package.json"))
				touch(t, filepath.Join(dir, "package-lock.json"))
			},
			want: true,
		},
		{
			name: "pnpm: marker newer",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "package.json"))
				touchAt(t, filepath.Join(dir, "pnpm-lock.yaml"), time.Now().Add(-time.Hour))
				touch(t, filepath.Join(dir, "node_modules", ".modules.yaml"))
			},
			want: false,
		},
		{
			name: "no lockfile and no node_modules",
			set: func(dir string) {
				touch(t, filepath.Join(dir, "package.json"))
			},
			want: true,
		},
		{
			name: "no lockfile but node_modules already populated",
			set: func(dir string) {
				touchAt(t, filepath.Join(dir, "package.json"), time.Now().Add(-time.Hour))
				touch(t, filepath.Join(dir, "node_modules", ".package-lock.json"))
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			c.set(dir)
			if got := jsNeedsInstall(dir); got != c.want {
				t.Errorf("jsNeedsInstall=%v want %v", got, c.want)
			}
		})
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	touchAt(t, path, time.Now())
}

func touchAt(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTree_CP(t *testing.T) {
	// Covers the cp fast path end-to-end when the host supports it.
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub/file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyTree(src, dst); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "sub/file"))
	if err != nil || string(got) != "x" {
		t.Fatalf("copied file: got %q err %v", got, err)
	}
}

// A no-op composer/npm install leaves the marker's mtime alone, so without a
// stamp the staleness check stays true forever and the watcher's rescan
// re-runs the install every minute for the life of the daemon. This is exactly
// what a worktree seeded with a copied vendor/ looks like: the tree is correct
// but its marker carries the main repo's older mtime.
func TestStampInstallMarker_makesAConvergedTreeStopAskingForInstall(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "composer.json"))
	touch(t, filepath.Join(dir, "composer.lock"))
	touchAt(t, filepath.Join(dir, "vendor", "composer", "installed.json"), time.Now().Add(-30*24*time.Hour))

	if !composerNeedsInstall(dir) {
		t.Fatal("precondition: a stale marker must ask for an install")
	}

	stampInstallMarker(filepath.Join(dir, "vendor", "composer", "installed.json"))

	if composerNeedsInstall(dir) {
		t.Error("after stamping, the marker still reports an install is needed")
	}
}

func TestStampInstallMarker_missingMarkerIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "vendor", "composer", "installed.json")

	stampInstallMarker(marker) // must not create it

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("stamping created a marker that no install had written")
	}
}

// jsProject writes the minimum for jsNeedsInstall to ask for an install, with
// the given lockfile contents so tests can vary the digest the memo keys on.
func jsProject(t *testing.T, lock string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// countJSInstalls replaces the JS installer with a stub that fails, and returns
// a counter of how many times it was actually reached.
func countJSInstalls(t *testing.T) *int {
	t.Helper()
	ResetJSInstallFailures()
	prev := jsInstaller
	attempts := 0
	jsInstaller = func(string, io.Writer) error {
		attempts++
		return errors.New("npm ci: exit status 1")
	}
	t.Cleanup(func() {
		jsInstaller = prev
		ResetJSInstallFailures()
	})
	return &attempts
}

// A lockfile that npm refuses is the same lockfile in every worktree seeded
// from it, and each attempt costs seconds. One bad package-lock.json must not
// multiply into one failed install per worktree.
func TestInstallDependencies_failedLockfileIsAttemptedOncePerPass(t *testing.T) {
	attempts := countJSInstalls(t)

	first := InstallDependencies(jsProject(t, `{"lockfileVersion":3}`), io.Discard)
	second := InstallDependencies(jsProject(t, `{"lockfileVersion":3}`), io.Discard)

	if *attempts != 1 {
		t.Errorf("ran the install %d times for one failing lockfile, want 1", *attempts)
	}
	if first == nil || second == nil {
		t.Error("both worktrees must report the failure; the skip is a shortcut, not a success")
	}
}

// The memo keys on the lockfile, not the project, so a worktree whose branch
// carries a different lockfile is still installed.
func TestInstallDependencies_differentLockfileIsStillAttempted(t *testing.T) {
	attempts := countJSInstalls(t)

	InstallDependencies(jsProject(t, `{"lockfileVersion":3}`), io.Discard)
	InstallDependencies(jsProject(t, `{"lockfileVersion":2}`), io.Discard)

	if *attempts != 2 {
		t.Errorf("ran the install %d times for two distinct lockfiles, want 2", *attempts)
	}
}

// The memo lasts one reconcile pass. A fixed lockfile, or a failure that was
// only the registry being unreachable, has to get another chance.
func TestResetJSInstallFailures_rearmsTheNextPass(t *testing.T) {
	attempts := countJSInstalls(t)

	InstallDependencies(jsProject(t, `{"lockfileVersion":3}`), io.Discard)
	ResetJSInstallFailures()
	InstallDependencies(jsProject(t, `{"lockfileVersion":3}`), io.Discard)

	if *attempts != 2 {
		t.Errorf("ran the install %d times across two passes, want 2", *attempts)
	}
}

// A genuinely stale tree must still be installed: stamping only ever happens
// after an install has run, never as a way to silence the check.
func TestStampInstallMarker_doesNotHideARealLockChange(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "composer.json"))
	touchAt(t, filepath.Join(dir, "vendor", "composer", "installed.json"), time.Now().Add(-time.Hour))
	stampInstallMarker(filepath.Join(dir, "vendor", "composer", "installed.json"))

	// The lock is edited after the install, as a branch switch or a require does.
	touch(t, filepath.Join(dir, "composer.lock"))

	if !composerNeedsInstall(dir) {
		t.Error("a lockfile written after the install must still require one")
	}
}
