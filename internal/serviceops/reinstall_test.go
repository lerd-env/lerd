package serviceops

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubReinstallSeams swaps the install/reprovision seams so the test exercises
// only ReinstallService's composition logic without hitting podman.
type reinstallRecorder struct {
	installCalls     []reinstallCall
	reprovCalls      []string
	familyRegenCalls []string
	installErr       error
	reprovErr        error
}
type reinstallCall struct {
	Name       string
	PresetName string
	Version    string
}

func stubReinstallSeams(t *testing.T) *reinstallRecorder {
	t.Helper()
	rec := &reinstallRecorder{}

	prevInstall := reinstallInstallFn
	prevReprov := reinstallReprovFn
	prevValidate := reinstallValidateFn
	prevPrefetch := reinstallPrefetchImageFn
	prevFamilyRegen := reinstallFamilyRegenFn
	reinstallInstallFn = func(name string, spec reinstallSpec, emit func(PhaseEvent)) (*config.CustomService, error) {
		rec.installCalls = append(rec.installCalls, reinstallCall{Name: name, PresetName: spec.presetName, Version: spec.version})
		emit(PhaseEvent{Phase: "starting_unit"})
		return &config.CustomService{Name: name}, rec.installErr
	}
	reinstallReprovFn = func(name string, emit func(PhaseEvent)) error {
		rec.reprovCalls = append(rec.reprovCalls, name)
		return rec.reprovErr
	}
	// Pre-flight checks talk to the real preset bundle and podman; the
	// composition tests don't care about them, so stub to permissive no-ops.
	reinstallValidateFn = func(string, reinstallSpec) error { return nil }
	reinstallPrefetchImageFn = func(string, func(PhaseEvent)) error { return nil }
	// Family regen would otherwise touch real podman / real preset bundle.
	reinstallFamilyRegenFn = func(name string) { rec.familyRegenCalls = append(rec.familyRegenCalls, name) }
	t.Cleanup(func() {
		reinstallInstallFn = prevInstall
		reinstallReprovFn = prevReprov
		reinstallValidateFn = prevValidate
		reinstallPrefetchImageFn = prevPrefetch
		reinstallFamilyRegenFn = prevFamilyRegen
	})
	return rec
}

func saveCustomServiceForReinstall(t *testing.T, name, version string) {
	t.Helper()
	svc := &config.CustomService{
		Name:          name,
		Image:         "docker.io/library/" + name + ":" + version,
		Family:        name,
		PresetVersion: version,
	}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}
}

func TestReinstallService_FailsIfNotInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	stubPodmanRemove(t)
	stubReinstallSeams(t)

	err := ReinstallService("unknown-svc", ReinstallOptions{}, func(PhaseEvent) {})
	if err == nil {
		t.Fatal("expected error reinstalling a service that was never installed")
	}
	if !strings.Contains(err.Error(), "not installed") && !strings.Contains(err.Error(), "no such") {
		t.Errorf("error should explain it's not installed, got %v", err)
	}
}

func TestReinstallService_PreservesVersionFromCustomServiceYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	stubPodmanRemove(t)
	rec := stubReinstallSeams(t)

	saveCustomServiceForReinstall(t, "myservice", "1.2.3")

	if err := ReinstallService("myservice", ReinstallOptions{}, func(PhaseEvent) {}); err != nil {
		t.Fatalf("ReinstallService: %v", err)
	}
	if len(rec.installCalls) != 1 {
		t.Fatalf("expected 1 install call, got %d (%v)", len(rec.installCalls), rec.installCalls)
	}
	if rec.installCalls[0].Name != "myservice" {
		t.Errorf("install name = %q, want myservice", rec.installCalls[0].Name)
	}
	if !strings.Contains(rec.installCalls[0].Version, "1.2.3") {
		t.Errorf("install version did not preserve original tag, got %q", rec.installCalls[0].Version)
	}
}

// A plain reinstall reprovisions too: state that went missing outside lerd is
// what the user is reinstalling to get back, and the creators check before they
// create, so a service whose data survived is left untouched.
func TestReinstallService_NoResetData_StillCallsReprovision(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	stubPodmanRemove(t)
	rec := stubReinstallSeams(t)
	saveCustomServiceForReinstall(t, "mariadb", "11.4")

	if err := ReinstallService("mariadb", ReinstallOptions{}, func(PhaseEvent) {}); err != nil {
		t.Fatalf("ReinstallService: %v", err)
	}
	if len(rec.reprovCalls) != 1 || rec.reprovCalls[0] != "mariadb" {
		t.Errorf("expected reprovision('mariadb'), got %v", rec.reprovCalls)
	}
}

func TestReinstallService_ResetData_CallsReprovision(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	stubPodmanRemove(t)
	rec := stubReinstallSeams(t)
	saveCustomServiceForReinstall(t, "postgres", "16")

	if err := ReinstallService("postgres", ReinstallOptions{ResetData: true}, func(PhaseEvent) {}); err != nil {
		t.Fatalf("ReinstallService: %v", err)
	}
	if len(rec.reprovCalls) != 1 || rec.reprovCalls[0] != "postgres" {
		t.Errorf("expected reprovision('postgres'), got %v", rec.reprovCalls)
	}
}

func TestReinstallService_EmitsReinstallStartingPhase(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	stubPodmanRemove(t)
	stubReinstallSeams(t)
	saveCustomServiceForReinstall(t, "mariadb", "11.4")

	var events []PhaseEvent
	if err := ReinstallService("mariadb", ReinstallOptions{}, func(e PhaseEvent) { events = append(events, e) }); err != nil {
		t.Fatalf("ReinstallService: %v", err)
	}
	if len(events) == 0 || events[0].Phase != "reinstall_starting" {
		t.Errorf("expected first phase reinstall_starting, got %v", events)
	}
}
