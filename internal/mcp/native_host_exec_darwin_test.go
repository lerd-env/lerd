package mcp

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// nativeHost points the install at the native runtime and records the host
// commands the tools would run instead of starting an FPM container.
func nativeHost(t *testing.T) (*[]*exec.Cmd, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	cfg.PHP.DefaultVersion = "8.4"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	php := filepath.Join(config.BinDir(), "php")
	if err := os.WriteFile(php, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var got []*exec.Cmd
	orig := runHostCmd
	runHostCmd = func(c *exec.Cmd) error { got = append(got, c); return nil }
	t.Cleanup(func() { runHostCmd = orig })
	return &got, php
}

func lastArgs(t *testing.T, got *[]*exec.Cmd, php string) []string {
	t.Helper()
	if len(*got) == 0 {
		t.Fatal("expected the command to run on the host, nothing ran there")
	}
	c := (*got)[len(*got)-1]
	if c.Path != php {
		t.Fatalf("ran %q, want lerd's php shim %q", c.Path, php)
	}
	return c.Args[1:]
}

func TestComposerRunsOnHostUnderNative(t *testing.T) {
	got, php := nativeHost(t)
	dir := t.TempDir()
	if _, rpcErr := execComposer(map[string]any{"path": dir, "args": []any{"install"}}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	args := lastArgs(t, got, php)
	if args[len(args)-1] != "install" {
		t.Errorf("args %v, want composer install", args)
	}
}

func TestComposerInstallForNewProjectRunsOnHostUnderNative(t *testing.T) {
	got, php := nativeHost(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runComposerInstallIfNeeded(dir, &out); err != nil {
		t.Fatal(err)
	}
	args := lastArgs(t, got, php)
	if !slices.Contains(args, "install") {
		t.Errorf("args %v, want composer install", args)
	}
}

func TestVendorRunRunsOnHostUnderNative(t *testing.T) {
	got, php := nativeHost(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "vendor", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "pest"), []byte("<?php"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, rpcErr := execVendorRun(map[string]any{"path": dir, "bin": "pest", "args": []any{"--version"}}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if args := lastArgs(t, got, php); !slices.Equal(args, []string{"vendor/bin/pest", "--version"}) {
		t.Errorf("args %v, want vendor/bin/pest --version", args)
	}
}

func TestArtisanRunsOnHostUnderNative(t *testing.T) {
	got, php := nativeHost(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "artisan"), []byte("<?php"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: dir, PHPVersion: "8.4", Framework: "laravel"}); err != nil {
		t.Fatal(err)
	}
	if _, rpcErr := execArtisan(map[string]any{"path": dir, "args": []any{"about"}}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if args := lastArgs(t, got, php); args[len(args)-1] != "about" {
		t.Errorf("args %v, want artisan about", args)
	}
}
