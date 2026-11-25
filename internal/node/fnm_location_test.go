package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fnmOnlyTheShellKnows puts an fnm and its data dir where only PATH and FNM_DIR
// lead, the way a terminal sees them, and returns both.
func fnmOnlyTheShellKnows(t *testing.T) (bin, dir string) {
	t.Helper()
	nodeHost(t, "fnm", false)
	tools, dir := t.TempDir(), filepath.Join(t.TempDir(), "my fnm")
	writeExec(t, tools, "fnm")
	for _, v := range []string{"v20.1.0", "v22.9.1"} {
		b := filepath.Join(dir, "node-versions", v, "installation", "bin")
		writeExec(t, b, "node")
		writeExec(t, b, "npm")
	}
	t.Setenv("PATH", tools)
	t.Setenv("FNM_DIR", dir)
	return filepath.Join(tools, "fnm"), dir
}

func recordFnm(t *testing.T) {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	RecordFnm(cfg)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
}

// The dashboard and the workers never load the user's shell, so they have to run
// the fnm, on the versions, that the terminal recorded.
func TestRecordFnm_DaemonsSeeWhatTheTerminalSaw(t *testing.T) {
	bin, dir := fnmOnlyTheShellKnows(t)
	recordFnm(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("FNM_DIR", "")

	m := ManagerByName("fnm")
	if !m.Available() || UserFnm() != bin {
		t.Fatalf("fnm not found without the shell's PATH, got %q", UserFnm())
	}
	if got := strings.Join(m.Command("22", "node", []string{"-v"}).Args, "|"); got != bin+"|--fnm-dir|"+dir+"|exec|--using=22|--|node|-v" {
		t.Errorf("Command = %s", got)
	}
	if got := m.ExecPrefix("22"); got != "'"+bin+"' --fnm-dir '"+dir+"' exec --using='22' --" {
		t.Errorf("ExecPrefix = %s", got)
	}
	if got := borrowedManagerBinDir("20"); !strings.HasPrefix(got, dir) || !strings.Contains(got, "v20.1.0") {
		t.Errorf("borrowed bin dir = %q, want v20.1.0 under %s", got, dir)
	}

	// A switch from the dashboard sees no FNM_DIR and must keep what was saved.
	recordFnm(t)
	if got := ManagerByName("fnm").ExecPrefix("22"); !strings.Contains(got, dir) {
		t.Errorf("recording without the shell lost the data dir: %s", got)
	}
}

// A recorded location that is gone falls back to the usual search.
func TestRecordFnm_StaleLocationIsIgnored(t *testing.T) {
	bin, dir := fnmOnlyTheShellKnows(t)
	recordFnm(t)
	t.Setenv("FNM_DIR", "")
	for _, p := range []string{bin, dir} {
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(os.Getenv("HOME"), ".cargo", "bin")
	writeExec(t, other, "fnm")
	if got := ManagerByName("fnm").ExecPrefix("22"); got != "'"+filepath.Join(other, "fnm")+"' exec --using='22' --" {
		t.Errorf("ExecPrefix = %s", got)
	}
}

// lerd's own fnm keeps its own versions, whatever the user's shell sets.
func TestRecordFnm_LeavesLerdsOwnAlone(t *testing.T) {
	fnmOnlyTheShellKnows(t)
	recordFnm(t)
	writeExec(t, config.BinDir(), "fnm")
	if got := ManagerByName("fnm").ExecPrefix("22"); got != "'"+lerdFnm()+"' exec --using='22' --" {
		t.Errorf("ExecPrefix = %s", got)
	}
}
