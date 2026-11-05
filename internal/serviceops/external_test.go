package serviceops

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func externalFixture(t *testing.T, version string, probeErr error) *[]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var started []string
	origProbe, origStart, origPull := externalProbeFn, externalStartFn, externalPullFn
	externalPullFn = func(string) error { return nil }
	externalProbeFn = func(*config.CustomService, string) (string, error) { return version, probeErr }
	externalStartFn = func(svc *config.CustomService) error {
		started = append(started, svc.Name)
		return nil
	}
	t.Cleanup(func() { externalProbeFn, externalStartFn, externalPullFn = origProbe, origStart, origPull })
	return &started
}

var externalReq = ExternalRequest{Name: "dbngin", Engine: "postgres", Host: "127.0.0.1", Port: 5433, User: "me", Password: "s3cret"}

func TestAddExternalDB_SavesAClientForTheServer(t *testing.T) {
	started := externalFixture(t, "17.2", nil)

	svc, err := AddExternalDB(externalReq)
	if err != nil {
		t.Fatal(err)
	}
	if svc.External == nil || svc.External.Host != "host.containers.internal" || svc.External.Port != 5433 {
		t.Errorf("external = %+v", svc.External)
	}
	if !strings.Contains(svc.Image, ":17-") {
		t.Errorf("a server on 17 needs a client on 17, got %s", svc.Image)
	}
	if len(svc.Ports) != 0 || svc.DataDir != "" {
		t.Errorf("a client holds no data and publishes nothing: %+v", svc)
	}
	if len(*started) != 1 {
		t.Errorf("started = %v", *started)
	}
	saved, err := config.LoadCustomService("dbngin")
	if err != nil || saved.External == nil || saved.Introspect == nil {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	raw, _ := os.ReadFile(filepath.Join(config.CustomServicesDir(), "dbngin.yaml"))
	if strings.Contains(string(raw), "s3cret") {
		t.Error("the password must stay out of the definition")
	}
	info, err := os.Stat(config.ExternalDBClientFile("dbngin"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("client file: %v %v", info, err)
	}
}

// A wrong host or password has to leave nothing to clean up.
func TestAddExternalDB_AFailedConnectionSavesNothing(t *testing.T) {
	started := externalFixture(t, "", errors.New("access denied"))

	_, err := AddExternalDB(externalReq)
	if err == nil || !strings.Contains(err.Error(), "access denied") || !strings.Contains(err.Error(), "could not connect") {
		t.Fatalf("got %v", err)
	}
	if config.CustomServiceExists("dbngin") || len(*started) != 0 {
		t.Error("nothing may be saved or started")
	}
	if _, err := os.Stat(config.ExternalDBClientFile("dbngin")); err == nil {
		t.Error("no client file may be left")
	}
}

func TestAddExternalDB_RefusesAPresetsName(t *testing.T) {
	externalFixture(t, "17.2", nil)
	req := externalReq
	req.Name = "redis"
	if _, err := AddExternalDB(req); err == nil || !strings.Contains(err.Error(), "redis") {
		t.Fatalf("got %v", err)
	}
}

func TestAddExternalDB_RefusesAnEngineItCannotSpeakTo(t *testing.T) {
	externalFixture(t, "7.2", nil)
	req := externalReq
	req.Engine = "redis"
	if _, err := AddExternalDB(req); err == nil {
		t.Fatal("redis has no database commands to reuse")
	}
}

func TestMatchingVersion(t *testing.T) {
	p, err := config.LoadPreset("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got := matchingVersion(p, "17.2"); got != "17" {
		t.Errorf("same major: %q", got)
	}
	if got := matchingVersion(p, "99.0"); got != "18" {
		t.Errorf("newer than any: %q", got)
	}
	m, err := config.LoadPreset("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if got := matchingVersion(m, "8.4.3"); got != "8.4" {
		t.Errorf("mysql: %q", got)
	}
}

func TestContainerHost(t *testing.T) {
	for in, want := range map[string]string{"127.0.0.1": "host.containers.internal", "localhost": "host.containers.internal", "[::1]": "host.containers.internal", "db.lan": "db.lan"} {
		if got := containerHost(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
}

// Removing the client must leave the server's data alone: no dump of it on the
// way out, whatever the caller asked for.
func TestRemoveService_NeverSnapshotsAnExternalServer(t *testing.T) {
	externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(externalReq); err != nil {
		t.Fatal(err)
	}
	origStatus, origContainer := removeUnitStatusFn, removeContainerFn
	removeUnitStatusFn = func(string) (string, error) { return "inactive", nil }
	removeContainerFn = func(string) {}
	t.Cleanup(func() { removeUnitStatusFn, removeContainerFn = origStatus, origContainer })
	var phases []string
	_ = RemoveService("dbngin", RemoveOptions{RemoveData: true}, func(e PhaseEvent) { phases = append(phases, e.Phase) })
	for _, p := range phases {
		if p == "snapshotting_data" || p == "removing_data" {
			t.Errorf("ran %s on an external server: %v", p, phases)
		}
	}
}

func TestExternalDBReachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if !ExternalDBReachable(config.ExternalDB{Host: "host.containers.internal", Port: port}) {
		t.Error("a listening server on this machine must read as up")
	}
	l.Close()
	if ExternalDBReachable(config.ExternalDB{Host: "host.containers.internal", Port: port}) {
		t.Error("a closed port must read as down")
	}
}

func TestUpdateExternalDBPassword_ReplacesItOnceItWorks(t *testing.T) {
	externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(externalReq); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExternalDBPassword("dbngin", "n3w"); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(config.ExternalDBPasswordFile("dbngin")); string(raw) != "n3w" {
		t.Errorf("password file = %q", raw)
	}
	if raw, _ := os.ReadFile(config.ExternalDBClientFile("dbngin")); !strings.Contains(string(raw), "n3w") {
		t.Errorf("client file = %q", raw)
	}
}

// A typo must not replace the password that still works.
func TestUpdateExternalDBPassword_KeepsTheOldOneWhenTheNewFails(t *testing.T) {
	externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(externalReq); err != nil {
		t.Fatal(err)
	}
	externalProbeFn = func(*config.CustomService, string) (string, error) { return "", errors.New("access denied") }
	err := UpdateExternalDBPassword("dbngin", "typo")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("got %v", err)
	}
	if raw, _ := os.ReadFile(config.ExternalDBPasswordFile("dbngin")); string(raw) != "s3cret" {
		t.Errorf("password file = %q", raw)
	}
}

func TestUpdateExternalDBPassword_RefusesAServiceLerdRuns(t *testing.T) {
	externalFixture(t, "17.2", nil)
	if err := config.SaveCustomService(&config.CustomService{Name: "ours", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExternalDBPassword("ours", "x"); err == nil {
		t.Fatal("a service lerd runs has no external password")
	}
}
