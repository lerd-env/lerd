package feedback

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnableVirtualTerminalRejectsNonConsole(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if enableVirtualTerminal(f) {
		t.Error("a regular file has no console mode, so virtual terminal processing must report off")
	}
}
