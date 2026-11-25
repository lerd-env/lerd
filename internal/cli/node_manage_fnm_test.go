package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// An install already on fnm still has to learn where the shell keeps it, and one
// whose location moved has to follow: "already using" must not skip that.
func TestRunNodeSetManager_RecordsFnmWhenAlreadyOnIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetNodeManager("fnm")
	cfg.SetNodeManaged(false)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		tools, dir := t.TempDir(), t.TempDir()
		bin := filepath.Join(tools, "fnm")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", tools)
		t.Setenv("FNM_DIR", dir)
		if err := runNodeSetManager(nil, []string{"fnm"}); err != nil {
			t.Fatal(err)
		}
		got, err := config.LoadGlobal()
		if err != nil {
			t.Fatal(err)
		}
		if got.Node.FnmBin != bin || got.Node.FnmDir != dir {
			t.Errorf("recorded %q in %q, want %q in %q", got.Node.FnmBin, got.Node.FnmDir, bin, dir)
		}
	}
}
