package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func stubGlanceRemove(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	prev := runGlanceRemove
	runGlanceRemove = func(args ...string) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { runGlanceRemove = prev })
	return &calls
}

func TestRemoveOmarchyGlance_RemovesAnInstalledPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config/omarchy/plugins/sh.lerd.glance"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := stubGlanceRemove(t)

	if found, err := removeOmarchyGlance(); !found || err != nil {
		t.Fatalf("removeOmarchyGlance = %v, %v, want a removal", found, err)
	}
	want := [][]string{{"omarchy-plugin-remove", "sh.lerd.glance", "--yes"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls = %v, want %v", *calls, want)
	}
}

func TestRemoveOmarchyGlance_NothingWithoutThePlugin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	calls := stubGlanceRemove(t)

	if found, _ := removeOmarchyGlance(); found {
		t.Error("removeOmarchyGlance claimed a removal with no plugin on disk")
	}
	if len(*calls) != 0 {
		t.Errorf("calls = %v, want none", *calls)
	}
}
