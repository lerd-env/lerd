package serviceops

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func fakeClients(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func externalFixture(t *testing.T, version string, probeErr error) ExternalRequest {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	origProbe := externalProbeFn
	externalProbeFn = func(*config.CustomService, string) (string, error) { return version, probeErr }
	t.Cleanup(func() { externalProbeFn = origProbe })
	req := externalReq
	req.Tools = fakeClients(t, "psql", "pg_dump")
	return req
}

var externalReq = ExternalRequest{Name: "dbngin", Engine: "postgres", Host: "127.0.0.1", Port: 5433, User: "me", Password: "s3cret"}

func TestAddExternalDB_SavesTheServerWithNoContainerBehindIt(t *testing.T) {
	req := externalFixture(t, "17.2", nil)

	svc, err := AddExternalDB(req)
	if err != nil {
		t.Fatal(err)
	}
	if svc.External == nil || svc.External.Host != "host.containers.internal" || svc.External.Port != 5433 {
		t.Errorf("external = %+v", svc.External)
	}
	if svc.External.Tools != req.Tools || svc.External.Version != "17.2" {
		t.Errorf("external = %+v", svc.External)
	}
	if svc.Image != "" || svc.Exec != "" || len(svc.Ports) != 0 || svc.DataDir != "" {
		t.Errorf("nothing runs here for an external server: %+v", svc)
	}
	saved, err := config.LoadCustomService("dbngin")
	if err != nil || saved.External == nil || saved.Introspect == nil {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	raw, _ := os.ReadFile(filepath.Join(config.CustomServicesDir(), "dbngin.yaml"))
	if strings.Contains(string(raw), "s3cret") {
		t.Error("the password must stay out of the definition")
	}
	info, err := os.Stat(filepath.Join(config.ServiceFilesDir("dbngin"), ".pgpass"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("client file: %v %v", info, err)
	}
}

func TestAddExternalDB_RefusesAFolderWithoutTheClients(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	req.Tools = fakeClients(t, "psql")
	_, err := AddExternalDB(req)
	if err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Fatalf("got %v, want the missing client named", err)
	}
	if config.CustomServiceExists("dbngin") {
		t.Error("a refused server must leave nothing behind")
	}
}

func TestAddExternalDB_NamesWhatItCouldNotFind(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	req.Tools = ""
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "")
	_, err := AddExternalDB(req)
	if err == nil || !strings.Contains(err.Error(), "psql and pg_dump") {
		t.Fatalf("got %v", err)
	}
}

func TestAddExternalDB_FindsTheClientsOnThePath(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	dir := req.Tools
	req.Tools = ""
	t.Setenv("PATH", dir)
	t.Setenv("SHELL", "")
	svc, err := AddExternalDB(req)
	if err != nil {
		t.Fatal(err)
	}
	if svc.External.Tools != dir {
		t.Errorf("tools = %q, want the folder saved so a later run needs no PATH", svc.External.Tools)
	}
}

func TestMysqlDumpFits(t *testing.T) {
	onlyMaria := fakeClients(t, "mariadb", "mariadb-dump")
	mysql := fakeClients(t, "mysql", "mysqldump")
	if err := mysqlDumpFits(onlyMaria, "8.4.3"); err == nil || !strings.Contains(err.Error(), "mysqldump") {
		t.Errorf("mariadb-dump against MySQL: %v", err)
	}
	if err := mysqlDumpFits(onlyMaria, "11.8.8-MariaDB"); err != nil {
		t.Errorf("mariadb-dump against MariaDB: %v", err)
	}
	if err := mysqlDumpFits(mysql, "8.4.3"); err != nil {
		t.Errorf("mysqldump against MySQL: %v", err)
	}
}

func TestAddExternalDB_LiftsAnEarlierRemove(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	if err := config.SetServiceRemoved("dbngin", true); err != nil {
		t.Fatal(err)
	}
	if _, err := AddExternalDB(req); err != nil {
		t.Fatal(err)
	}
	if config.ServiceIsRemoved("dbngin") {
		t.Error("a server added again must not stay marked removed")
	}
}

func TestPgDumpTooOld(t *testing.T) {
	for _, c := range []struct {
		dump, server string
		want         bool
	}{
		{"pg_dump (PostgreSQL) 16.3", "18.1", true},
		{"pg_dump (PostgreSQL) 18.6", "18.1", false},
		{"pg_dump (PostgreSQL) 18.6", "16.2 (Debian 16.2-1)", false},
	} {
		if got := pgDumpTooOld(c.dump, c.server); got != c.want {
			t.Errorf("%q vs %q = %v", c.dump, c.server, got)
		}
	}
}

// A wrong host or password has to leave nothing to clean up.
func TestAddExternalDB_AFailedConnectionSavesNothing(t *testing.T) {
	req := externalFixture(t, "", errors.New("access denied"))

	_, err := AddExternalDB(req)
	if err == nil || !strings.Contains(err.Error(), "access denied") || !strings.Contains(err.Error(), "could not connect") {
		t.Fatalf("got %v", err)
	}
	if config.CustomServiceExists("dbngin") {
		t.Error("nothing may be saved")
	}
	if _, err := os.Stat(filepath.Join(config.ServiceFilesDir("dbngin"), ".pgpass")); err == nil {
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
	req := externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(req); err != nil {
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

func TestUpdateExternalDB_ReplacesItOnceItWorks(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(req); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExternalDB("dbngin", "n3w", ""); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(config.ExternalDBPasswordFile("dbngin")); string(raw) != "n3w" {
		t.Errorf("password file = %q", raw)
	}
	if raw, _ := os.ReadFile(filepath.Join(config.ServiceFilesDir("dbngin"), ".pgpass")); !strings.Contains(string(raw), "n3w") {
		t.Errorf("client file = %q", raw)
	}
}

// A typo must not replace the password that still works.
func TestUpdateExternalDB_KeepsTheOldOneWhenTheNewFails(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(req); err != nil {
		t.Fatal(err)
	}
	externalProbeFn = func(*config.CustomService, string) (string, error) { return "", errors.New("access denied") }
	err := UpdateExternalDB("dbngin", "typo", "")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("got %v", err)
	}
	if raw, _ := os.ReadFile(config.ExternalDBPasswordFile("dbngin")); string(raw) != "s3cret" {
		t.Errorf("password file = %q", raw)
	}
}

// Changing only the folder must not lose the password.
func TestUpdateExternalDB_ChangesTheToolsAndKeepsThePassword(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(req); err != nil {
		t.Fatal(err)
	}
	moved := fakeClients(t, "psql", "pg_dump")
	var tried string
	externalProbeFn = func(_ *config.CustomService, password string) (string, error) {
		tried = password
		return "17.2", nil
	}
	if err := UpdateExternalDB("dbngin", "", moved); err != nil {
		t.Fatal(err)
	}
	if tried != "s3cret" {
		t.Errorf("connected with %q, want the saved password", tried)
	}
	saved, _ := config.LoadCustomService("dbngin")
	if saved.External.Tools != moved {
		t.Errorf("tools = %q", saved.External.Tools)
	}
}

func TestUpdateExternalDB_RefusesAFolderWithoutTheClients(t *testing.T) {
	req := externalFixture(t, "17.2", nil)
	if _, err := AddExternalDB(req); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExternalDB("dbngin", "", t.TempDir()); err == nil {
		t.Fatal("want a refusal")
	}
	saved, _ := config.LoadCustomService("dbngin")
	if saved.External.Tools != req.Tools {
		t.Errorf("tools changed to %q", saved.External.Tools)
	}
}

func TestExternalDBStatus(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	e := config.ExternalDB{Host: "host.containers.internal", Port: port}
	if got := ExternalDBStatus(e); got != "active" {
		t.Errorf("listening: %q", got)
	}
	ln.Close()
	if got := ExternalDBStatus(e); got != "offline" {
		t.Errorf("closed: %q", got)
	}
}

func TestExternalShell_RunsOnTheMachineWithTheClientFileAsHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	svc := &config.CustomService{Name: "dbngin", Family: "mysql", External: &config.ExternalDB{Host: "h", Tools: "/opt/tools"}}
	out, err := ExternalShell(context.Background(), svc, `printf "%s %s" "$HOME" "$PATH"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := config.ServiceFilesDir("dbngin") + " /opt/tools:/usr/bin:/bin"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestUpdateExternalDB_RefusesAServiceLerdRuns(t *testing.T) {
	externalFixture(t, "17.2", nil)
	if err := config.SaveCustomService(&config.CustomService{Name: "ours", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExternalDB("ours", "x", ""); err == nil {
		t.Fatal("a service lerd runs has no external password")
	}
}
