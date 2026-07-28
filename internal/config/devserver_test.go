package config

import (
	"os"
	"path/filepath"
	"testing"
)

func viteProject(t *testing.T, devScript string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := `{"scripts":{"dev":"` + devScript + `"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDevServerToolForFollowsAPackageScript(t *testing.T) {
	dir := viteProject(t, "vite")
	tool := DevServerToolFor(dir, "npm run dev")
	if tool == nil || tool.Name != "vite" {
		t.Fatalf("DevServerToolFor() = %v, want the vite tool", tool)
	}
}

func TestDevServerToolForMatchesADirectInvocation(t *testing.T) {
	dir := viteProject(t, "vite")
	if tool := DevServerToolFor(dir, "npx vite"); tool == nil {
		t.Fatal("a direct npx invocation should match")
	}
}

// The flags lerd appends are handed to whatever the command starts. A runner
// that reaches vite later in the line would get them instead, so it must not
// match.
func TestDevServerToolForIgnoresARunnerWrappingTheTool(t *testing.T) {
	dir := viteProject(t, `concurrently "vite" "php artisan queue:work"`)
	if tool := DevServerToolFor(dir, "npm run dev"); tool != nil {
		t.Fatalf("DevServerToolFor() = %v, want nil for a runner-wrapped command", tool)
	}
}

// A project that does not have the tool installed is not running it, whatever
// its scripts happen to say.
func TestDevServerToolForRequiresTheToolInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if tool := DevServerToolFor(dir, "npm run dev"); tool != nil {
		t.Fatal("no node_modules/vite, so nothing should match")
	}
}

func TestDevServerToolForIgnoresAnUnrelatedCommand(t *testing.T) {
	dir := viteProject(t, "vite")
	if tool := DevServerToolFor(dir, "php artisan queue:work"); tool != nil {
		t.Fatal("a queue worker should never be treated as a dev server")
	}
}
