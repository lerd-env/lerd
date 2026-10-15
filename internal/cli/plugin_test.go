package cli

import (
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
