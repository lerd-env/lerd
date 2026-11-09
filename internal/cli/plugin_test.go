package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/plugins"
	"github.com/spf13/cobra"
)

func isolatePlugins(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestAddPluginCommands_RegistersAndKeepsBuiltins(t *testing.T) {
	isolatePlugins(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "lerd"}
	builtin := &cobra.Command{Use: "hello:greet", Short: "lerd's own"}
	root.AddCommand(builtin)

	AddPluginCommands(root)

	var count int
	for _, c := range root.Commands() {
		if c.Name() == "hello:greet" {
			count++
			if c != builtin {
				t.Error("a plugin command must not replace a built-in of the same name")
			}
		}
	}
	if count != 1 {
		t.Fatalf("want exactly one hello:greet, got %d", count)
	}
}

func TestAddPluginCommands_AddsPluginCommand(t *testing.T) {
	isolatePlugins(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "lerd"}
	AddPluginCommands(root)
	if !commandExists(root, "hello:greet") {
		t.Fatal("hello:greet should be registered from the plugin manifest")
	}
}

// pluginExitHelperEnv makes TestPluginCommand_ExitHelper run lerd's plugin
// commands in a child process, since they end it with os.Exit. config's test
// guard treats the dirs a process starts with as real, so the child gets decoys.
const pluginExitHelperEnv = "LERD_TEST_PLUGIN_EXIT_HELPER"

func TestPluginCommand_ExitHelper(t *testing.T) {
	if os.Getenv(pluginExitHelperEnv) == "" {
		t.Skip("run by TestPluginCommand_ExpectedExitIsNotAFailure")
	}
	t.Setenv("XDG_DATA_HOME", os.Getenv(pluginExitHelperEnv+"_DATA"))
	t.Setenv("XDG_CONFIG_HOME", os.Getenv(pluginExitHelperEnv+"_CONFIG"))
	root := &cobra.Command{Use: "lerd"}
	AddPluginCommands(root)
	root.SetArgs([]string{"cargo:test", os.Getenv(pluginExitHelperEnv)})
	_ = root.Execute()
}

// A command's own answer, such as failing tests, passes its exit code on
// without blaming the plugin; any other exit is still reported.
func TestPluginCommand_ExpectedExitIsNotAFailure(t *testing.T) {
	isolatePlugins(t)
	dir := filepath.Join(plugins.Dir(), "cargo")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "api_version: 1\nname: cargo\ncommands:\n  - {name: test, run: bin/test, expected_exits: [101]}\n"
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "test"), []byte("#!/bin/sh\necho 'test result: FAILED' >&2\nexit \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(code string) int {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestPluginCommand_ExitHelper$", "-test.timeout=60s")
		cmd.Env = append(os.Environ(),
			pluginExitHelperEnv+"="+code,
			pluginExitHelperEnv+"_DATA="+os.Getenv("XDG_DATA_HOME"),
			pluginExitHelperEnv+"_CONFIG="+os.Getenv("XDG_CONFIG_HOME"),
			"XDG_DATA_HOME="+t.TempDir(),
			"XDG_CONFIG_HOME="+t.TempDir(),
		)
		var exit *exec.ExitError
		if err := cmd.Run(); !errors.As(err, &exit) {
			t.Fatalf("lerd cargo:test %s should exit non-zero, got %v", code, err)
		}
		return exit.ExitCode()
	}

	if got := run("101"); got != 101 {
		t.Fatalf("exit code = %d, want the command's 101", got)
	}
	if f, ok := plugins.LastFailure("cargo"); ok {
		t.Fatalf("an expected exit was recorded as a plugin failure: %+v", f)
	}
	if got := run("3"); got != 3 {
		t.Fatalf("exit code = %d, want 3", got)
	}
	if _, ok := plugins.LastFailure("cargo"); !ok {
		t.Error("an exit the command did not declare should still be recorded")
	}
}
