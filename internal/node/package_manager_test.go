package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageManager_fromLockfile(t *testing.T) {
	cases := []struct {
		name string
		file string
		want string
	}{
		{"pnpm", "pnpm-lock.yaml", "pnpm"},
		{"yarn", "yarn.lock", "yarn"},
		{"bun", "bun.lockb", "bun"},
		{"npm", "package-lock.json", "npm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
			if got := PackageManager(dir); got != tc.want {
				t.Errorf("PackageManager(%s) = %q, want %q", tc.file, got, tc.want)
			}
		})
	}
}

func TestPackageManager_emptyDirDefaultsToNpm(t *testing.T) {
	if got := PackageManager(t.TempDir()); got != "npm" {
		t.Errorf("PackageManager(empty) = %q, want npm", got)
	}
}

func TestPackageManager_packageManagerFieldWins(t *testing.T) {
	// The corepack pin beats a stray lockfile from another manager.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"packageManager":"pnpm@9.1.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if got := PackageManager(dir); got != "pnpm" {
		t.Errorf("PackageManager(packageManager=pnpm) = %q, want pnpm", got)
	}
}
