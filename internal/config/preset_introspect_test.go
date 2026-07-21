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
	q := p.Introspect.ListDatabases
	if q == "" {
		t.Fatal("the postgres preset declares no list_databases query")
	}
	if !strings.Contains(q, "template1") {
		t.Errorf("query reports the catalog baseline as data: %s", q)
	}
	if !strings.Contains(q, "GREATEST") {
		t.Errorf("query can report a negative size for a database below the baseline: %s", q)
	}
}
