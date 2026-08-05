package config

import (
	"strings"
	"testing"
)

// Every fresh postgres database inherits about 7.5 MB of system catalogs from
// template1, so a raw pg_database_size makes an empty database look like data.
// The mysql query already reports table data only; postgres has to net the
// template baseline off to say the same thing.
func TestPostgresListDatabases_NetsOffTheTemplateBaseline(t *testing.T) {
	p, err := LoadPreset("postgres")
	if err != nil {
		t.Fatalf("loading the postgres preset: %v", err)
	}
	spec := p.Introspect.DatabasesEntity()
	if spec == nil {
		t.Fatal("the postgres preset declares no databases entity")
	}
	q := spec.List
	if !strings.Contains(q, "template1") {
		t.Errorf("query reports the catalog baseline as data: %s", q)
	}
	if !strings.Contains(q, "GREATEST") {
		t.Errorf("query can report a negative size for a database below the baseline: %s", q)
	}
}

// The mysql client falls back to a unix socket when no host is given, and the
// path it compiles in is not always the one the server listens on, so every
// action has to name the host the way the migrate path already does.
func TestMysqlDatabaseActionsAddressTheClientByHost(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("loading the mysql preset: %v", err)
	}
	spec := p.Introspect.DatabasesEntity()
	if spec == nil {
		t.Fatal("the mysql preset declares no databases entity")
	}
	cmds := map[string]string{"list": spec.List}
	for name, act := range spec.Actions {
		cmds[name] = act.Exec
	}
	for name, cmd := range cmds {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		if !strings.Contains(cmd, "-h 127.0.0.1") {
			t.Errorf("%s relies on socket resolution: %s", name, cmd)
		}
	}
}
