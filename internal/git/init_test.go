package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_createsRepoWithBranch(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !IsMainRepo(dir) {
		t.Fatal("expected .git directory after Init")
	}
	if MainBranch(dir) == "" {
		t.Error("expected a branch after Init")
	}
}

// A site in a monorepo subfolder already has git; a nested repo would hide it.
func TestInit_refusesInsideExistingRepo(t *testing.T) {
	parent, _ := initRepo(t)
	sub := filepath.Join(parent, "app")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Init(sub)
	if err == nil || !strings.Contains(err.Error(), "already inside a git repository") {
		t.Fatalf("want refusal, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(sub, ".git")); !os.IsNotExist(statErr) {
		t.Error("nested .git was created")
	}
}

// A folder the parent repo ignores is not under its git, so it gets its own.
func TestInit_allowsFolderIgnoredByParent(t *testing.T) {
	parent, _ := initRepo(t)
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte("sites\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(parent, "sites", "shop")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := EnclosingRepo(sub); ok {
		t.Error("ignored folder reported as inside the parent repo")
	}
	if err := Init(sub); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !IsMainRepo(sub) {
		t.Error("expected a repo of its own")
	}
}
