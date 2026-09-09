package config

import (
	"os"
	"path/filepath"
	"testing"
)

// installedVite makes the project look like it has vite, which is what every
// dev server decision is gated on first.
func installedVite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A framework that starts vite through its own console command is a vite
// project however little the command says so, and the declaration is what says
// so instead.
func TestDevServerToolForWorker_DeclarationBeatsTheCommand(t *testing.T) {
	dir := installedVite(t)
	w := FrameworkWorker{
		Command:   "php artisan vite:watch theme-vampire",
		Host:      true,
		DevServer: &WorkerDevServer{Tool: "vite"},
	}

	if got := DevServerToolForWorker(dir, w, w.Command); got == nil || got.Name != "vite" {
		t.Errorf("tool = %v, want the declared vite", got)
	}
	// Without the declaration the same command says nothing, which is the state
	// this was reported from.
	plain := FrameworkWorker{Command: w.Command, Host: true}
	if got := DevServerToolForWorker(dir, plain, plain.Command); got != nil {
		t.Errorf("tool = %v, want nothing for an undeclared indirect command", got)
	}
}

// A declaration naming something lerd has no integration for is not a licence
// to treat the worker as a dev server.
func TestDevServerToolForWorker_UnknownToolIsNotMatched(t *testing.T) {
	dir := installedVite(t)
	w := FrameworkWorker{Command: "php artisan encore:watch", Host: true, DevServer: &WorkerDevServer{Tool: "encore"}}

	if got := DevServerToolForWorker(dir, w, w.Command); got != nil {
		t.Errorf("tool = %v, want nothing for a tool lerd does not handle", got)
	}
}

// A worker with no declaration keeps the old behaviour, so a plain `npm run dev`
// is still recognised.
func TestDevServerToolForWorker_FallsBackToTheCommand(t *testing.T) {
	dir := installedVite(t)
	if err := os.WriteFile(filepath.Join(dir, "package.json"),
		[]byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w := FrameworkWorker{Command: "npm run dev", Host: true}

	if got := DevServerToolForWorker(dir, w, w.Command); got == nil {
		t.Error("tool = nil, want vite from the command")
	}
}
