package git

import (
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
