package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFrankenPHPHintsEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := DetectFrankenPHPHints(dir); len(got) != 0 {
		t.Fatalf("empty dir should yield no hints, got %v", got)
	}
}

func TestDetectFrankenPHPHintsLaravelOctane(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "composer.json", `{"require":{"laravel/octane":"^2.0"}}`)
	write(t, dir, ".env", "APP_ENV=local\nOCTANE_SERVER=frankenphp\n")
	hints := DetectFrankenPHPHints(dir)
	if len(hints) != 1 || hints[0].Signal != "laravel-octane-franken" {
		t.Fatalf("want laravel-octane-franken, got %+v", hints)
	}
}

func TestDetectFrankenPHPHintsLaravelOctaneSwapped(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "composer.json", `{"require":{"laravel/octane":"^2.0"}}`)
	write(t, dir, ".env", "OCTANE_SERVER=swoole\n")
	if got := DetectFrankenPHPHints(dir); len(got) != 0 {
		t.Fatalf("octane with swoole should not flag frankenphp, got %+v", got)
	}
}

func TestDetectFrankenPHPHintsSymfonyRuntime(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "composer.json", `{"require":{"runtime/frankenphp-symfony":"^1.0","runtime/frankenphp":"^1.0"}}`)
	hints := DetectFrankenPHPHints(dir)
	if len(hints) != 1 || hints[0].Signal != "symfony-runtime" {
		t.Fatalf("symfony runtime should dedupe runtime-frankenphp, got %+v", hints)
	}
}

func TestDetectFrankenPHPHintsContainerfile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Containerfile.lerd", "FROM dunglas/frankenphp:php8.4-alpine\n")
	hints := DetectFrankenPHPHints(dir)
	if len(hints) != 1 || hints[0].Signal != "containerfile" {
		t.Fatalf("containerfile signal missing: %+v", hints)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
