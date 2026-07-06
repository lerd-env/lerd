package podman

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fpmTestEnv points config at a temp dir and stubs bindability so the shift
// guard is driven by an explicit busy-port set rather than real sockets.
func fpmTestEnv(t *testing.T, busy ...int) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	set := map[int]bool{}
	for _, p := range busy {
		set[p] = true
	}
	orig := fpmPortsBindable
	fpmPortsBindable = func(p int) bool { return !set[p] }
	t.Cleanup(func() { fpmPortsBindable = orig })
}

func TestSetFPMPorts_PersistsMappings(t *testing.T) {
	fpmTestEnv(t)
	got, err := SetFPMPorts("8.3", []string{"3000:3000", "5173:5173"})
	if err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	if len(got) != 2 || got[0] != "3000:3000" || got[1] != "5173:5173" {
		t.Fatalf("resolved = %v, want [3000:3000 5173:5173]", got)
	}
	if stored := config.FPMPortsFor("8.3"); len(stored) != 2 {
		t.Errorf("stored = %v, want 2 entries", stored)
	}
}

func TestSetFPMPorts_ShiftsWhenHostPortBusy(t *testing.T) {
	fpmTestEnv(t, 3000) // 3000 already bound on the host
	got, err := SetFPMPorts("8.3", []string{"3000:3000"})
	if err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	if len(got) != 1 || got[0] != "3001:3000" {
		t.Errorf("resolved = %v, want [3001:3000] (shifted off busy 3000)", got)
	}
}

func TestSetFPMPorts_ShiftsOffAnotherServicePort(t *testing.T) {
	fpmTestEnv(t)
	// A service already claims 3000; the guard must relocate the FPM mapping.
	cfg, _ := config.LoadGlobal()
	cfg.Services = map[string]config.ServiceConfig{"mysql": {Port: 3000}}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got, err := SetFPMPorts("8.3", []string{"3000:3000"})
	if err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	if len(got) != 1 || got[0] != "3001:3000" {
		t.Errorf("resolved = %v, want [3001:3000]", got)
	}
}

// Re-saving an unchanged list must keep every port where it is: this version's
// own published port is not a collision with itself even when the probe reports
// it busy (the running container holds it, then frees it on restart).
func TestSetFPMPorts_KeepsOwnPortOnResave(t *testing.T) {
	fpmTestEnv(t, 3000) // 3000 "busy" because this version's FPM already binds it
	cfg, _ := config.LoadGlobal()
	cfg.PHP.FPMPorts = map[string][]string{"8.3": {"3000:3000"}}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got, err := SetFPMPorts("8.3", []string{"3000:3000"})
	if err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	if len(got) != 1 || got[0] != "3000:3000" {
		t.Errorf("resolved = %v, want [3000:3000] (kept own port)", got)
	}
}

func TestSetFPMPorts_ShiftsDuplicateWithinBatch(t *testing.T) {
	fpmTestEnv(t)
	got, err := SetFPMPorts("8.3", []string{"3000:3000", "3000:8080"})
	if err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	if len(got) != 2 || got[0] != "3000:3000" || got[1] != "3001:8080" {
		t.Errorf("resolved = %v, want [3000:3000 3001:8080]", got)
	}
}

// An exact-duplicate host:container spec in the batch collapses to a single
// mapping rather than being shifted onto a redundant extra host port.
func TestSetFPMPorts_CollapsesExactDuplicate(t *testing.T) {
	fpmTestEnv(t)
	got, err := SetFPMPorts("8.3", []string{"3000:3000", "3000:3000"})
	if err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	if len(got) != 1 || got[0] != "3000:3000" {
		t.Errorf("resolved = %v, want [3000:3000] (duplicate collapsed)", got)
	}
}

func TestSetFPMPorts_EmptyClearsVersion(t *testing.T) {
	fpmTestEnv(t)
	if _, err := SetFPMPorts("8.3", []string{"3000:3000"}); err != nil {
		t.Fatalf("seed SetFPMPorts: %v", err)
	}
	if _, err := SetFPMPorts("8.3", nil); err != nil {
		t.Fatalf("clear SetFPMPorts: %v", err)
	}
	if got := config.FPMPortsFor("8.3"); got != nil {
		t.Errorf("after clear, FPMPortsFor(8.3) = %v, want nil", got)
	}
}

func TestSetFPMPorts_RejectsBareHostPort(t *testing.T) {
	fpmTestEnv(t)
	if _, err := SetFPMPorts("8.3", []string{"3000"}); err == nil {
		t.Error("expected an error for a bare host port with no container port")
	}
}

func TestAddFPMPort_ReturnsShiftedHost(t *testing.T) {
	fpmTestEnv(t, 3000)
	got, err := AddFPMPort("8.3", 3000, 3000)
	if err != nil {
		t.Fatalf("AddFPMPort: %v", err)
	}
	if got != 3001 {
		t.Errorf("AddFPMPort returned host %d, want 3001", got)
	}
}

func TestRemoveFPMPort_DropsMapping(t *testing.T) {
	fpmTestEnv(t)
	if _, err := SetFPMPorts("8.3", []string{"3000:3000", "5173:5173"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := RemoveFPMPort("8.3", 3000); err != nil {
		t.Fatalf("RemoveFPMPort: %v", err)
	}
	got := config.FPMPortsFor("8.3")
	if len(got) != 1 || got[0] != "5173:5173" {
		t.Errorf("after remove = %v, want [5173:5173]", got)
	}
}

// The version's FPM ports must reach the shared quadlet as loopback-bound
// PublishPort lines (LAN off) — mirroring exactly what WriteFPMQuadlet then
// WriteQuadletDiff do: render, ApplyExtraPorts, BindForLAN.
func TestFPMPortsRenderAsLoopbackPublish(t *testing.T) {
	fpmTestEnv(t)
	if _, err := SetFPMPorts("8.3", []string{"3000:3000"}); err != nil {
		t.Fatalf("SetFPMPorts: %v", err)
	}
	content, err := renderFPMQuadletContent("8.3")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	content = ApplyExtraPorts(content, config.FPMPortsFor("8.3"))
	loopback := BindForLAN(content, false)
	if !strings.Contains(loopback, "PublishPort=127.0.0.1:3000:3000") {
		t.Errorf("expected loopback-bound FPM publish line, got:\n%s", loopback)
	}
	lan := BindForLAN(content, true)
	if !strings.Contains(lan, "PublishPort=3000:3000") {
		t.Errorf("expected bare LAN publish line, got:\n%s", lan)
	}
}
