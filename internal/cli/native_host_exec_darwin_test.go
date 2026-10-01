package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// captureHostExec swaps the host runner for one that records the command it
// would have run, so a test can see the host path was taken without a real php.
func captureHostExec(t *testing.T) *[]*exec.Cmd {
	t.Helper()
	var got []*exec.Cmd
	orig := runHostExec
	runHostExec = func(c *exec.Cmd) error { got = append(got, c); return nil }
	t.Cleanup(func() { runHostExec = orig })
	return &got
}

// The shim dir has to hold a php for the host path to resolve one; a real
// install writes it, the test writes a stand-in.
func fakeShimPHP(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	php := filepath.Join(config.BinDir(), "php")
	if err := os.WriteFile(php, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return php
}

func TestSetupStepsRunOnHostUnderNative(t *testing.T) {
	nativeMode(t)
	php := fakeShimPHP(t)
	dir := t.TempDir()
	cases := []struct {
		name string
		run  func() error
		args []string
	}{
		{"composer install", func() error { return composerInContainer(dir, "install") }, []string{"install"}},
		{"framework setup command", func() error { return execInContainer(dir, "php artisan storage:link") }, []string{"artisan", "storage:link"}},
		{"console command", func() error { return consoleIn(dir, "artisan", "key:generate") }, []string{"artisan", "key:generate"}},
	}
	for _, c := range cases {
		got := captureHostExec(t)
		if err := c.run(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(*got) != 1 {
			t.Fatalf("%s: expected one host run, got %d", c.name, len(*got))
		}
		cmd := (*got)[0]
		if cmd.Path != php {
			t.Errorf("%s: ran %q, want lerd's php shim %q", c.name, cmd.Path, php)
		}
		if cmd.Dir != dir {
			t.Errorf("%s: dir %q, want %q", c.name, cmd.Dir, dir)
		}
		if !slices.Equal(cmd.Args[len(cmd.Args)-len(c.args):], c.args) {
			t.Errorf("%s: args %v, want to end with %v", c.name, cmd.Args, c.args)
		}
	}
}

// A setup command need not start with php (CakePHP's bin/cake); it runs as
// written, with the project's vendor/bin and lerd's shim dir on its PATH so the
// php it calls is the native one.
func TestSetupScriptRunsOnHostWithShimPath(t *testing.T) {
	nativeMode(t)
	fakeShimPHP(t)
	dir := t.TempDir()
	got := captureHostExec(t)
	if err := execInContainer(dir, "bin/cake migrations migrate"); err != nil {
		t.Fatal(err)
	}
	cmd := (*got)[0]
	if cmd.Path != "bin/cake" || cmd.Dir != dir {
		t.Errorf("ran %q in %q, want bin/cake in %q", cmd.Path, cmd.Dir, dir)
	}
	var path string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "PATH=") {
			path = e
		}
	}
	want := "PATH=" + filepath.Join(dir, "vendor", "bin") + string(os.PathListSeparator) + config.BinDir()
	if !strings.HasPrefix(path, want) {
		t.Errorf("PATH %q, want it to start with %q", path, want)
	}
}

func TestHostExecFailureIsReported(t *testing.T) {
	nativeMode(t)
	fakeShimPHP(t)
	orig := runHostExec
	runHostExec = func(*exec.Cmd) error { return &exec.ExitError{} }
	t.Cleanup(func() { runHostExec = orig })
	if err := execInContainer(t.TempDir(), "php artisan migrate"); err == nil {
		t.Error("a failing host command must fail the step")
	}
}
