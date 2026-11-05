package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeTools(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestMissingExternalTools(t *testing.T) {
	cases := []struct {
		name, family string
		tools        []string
		want         string
	}{
		{"mysql pair", "mysql", []string{"mysql", "mysqldump"}, ""},
		{"mariadb names", "mariadb", []string{"mariadb", "mariadb-dump"}, ""},
		{"postgres pair", "postgres", []string{"psql", "pg_dump"}, ""},
		{"no dump tool", "mysql", []string{"mysql"}, "mysqldump"},
		{"no client", "postgres", []string{"pg_dump"}, "psql"},
		{"empty folder", "postgres", nil, "psql, pg_dump"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(MissingExternalTools(c.family, fakeTools(t, c.tools...)), ", "); got != c.want {
				t.Errorf("missing = %q, want %q", got, c.want)
			}
		})
	}
}

func TestMissingExternalTools_ANonExecutableFileDoesNotCount(t *testing.T) {
	dir := fakeTools(t, "psql", "pg_dump")
	if err := os.Chmod(filepath.Join(dir, "pg_dump"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := MissingExternalTools("postgres", dir); len(got) != 1 {
		t.Errorf("missing = %v, want pg_dump", got)
	}
}

func TestLocateExternalTools(t *testing.T) {
	half := fakeTools(t, "psql")
	full := fakeTools(t, "psql", "pg_dump")
	if got := LocateExternalTools("postgres", []string{"", half, full}); got != full {
		t.Errorf("found %q, want the first folder holding both tools", got)
	}
	if got := LocateExternalTools("postgres", []string{half}); got != "" {
		t.Errorf("found %q, want nothing", got)
	}
}

func TestLocateExternalTools_SkipsLerdsOwnShims(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	shims := BinDir()
	if err := os.MkdirAll(shims, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"mysql", "mysqldump"} {
		if err := os.WriteFile(filepath.Join(shims, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// These run inside lerd's container, which is the very thing this avoids.
	if got := LocateExternalTools("mysql", []string{shims}); got != "" {
		t.Errorf("found lerd's shims at %q", got)
	}
}

func TestExternalDBToolEnv(t *testing.T) {
	t.Setenv("PATH", BinDir()+":/usr/bin")
	t.Setenv("HOME", "/home/someone")
	svc := &CustomService{Family: "postgres", External: &ExternalDB{Host: "host.containers.internal", Port: 5433, User: "me", Tools: "/opt/pg/bin"}}
	env := strings.Join(ExternalDBToolEnv(svc, "/secrets"), "\n")
	for _, want := range []string{"PGHOST=127.0.0.1", "PGPORT=5433", "PGUSER=me", "PGDATABASE=postgres", "HOME=/secrets", "PATH=/opt/pg/bin:/usr/bin:/bin"} {
		if !strings.Contains(env, want) {
			t.Errorf("env lacks %s:\n%s", want, env)
		}
	}
	if strings.Contains(env, "HOME=/home/someone") || strings.Contains(env, BinDir()) {
		t.Errorf("the real home or lerd's shims leaked in:\n%s", env)
	}
}

func TestExternalDB_LocalHost(t *testing.T) {
	if got := (ExternalDB{Host: "host.containers.internal"}).LocalHost(); got != "127.0.0.1" {
		t.Errorf("got %q, the machine's own tools cannot resolve the container name", got)
	}
	if got := (ExternalDB{Host: "db.internal"}).LocalHost(); got != "db.internal" {
		t.Errorf("got %q", got)
	}
}
