package config

import (
	"os"
	"os/exec"
	"path/filepath"
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

// MySQL 9.7 runs with GTIDs on, so its dumps carry a GTID_PURGED statement that
// fails against the server they came from, after the restore has already emptied
// the database. New dumps leave it out, and the import strips it from old ones.
func TestMysqlDumpsRestoreOntoAGTIDServer(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("loading the mysql preset: %v", err)
	}
	spec := p.Introspect.DatabasesEntity()
	for _, name := range []string{"export", "export_all"} {
		if !strings.Contains(spec.Actions[name].Exec, "--set-gtid-purged=OFF") {
			t.Errorf("%s writes the GTID_PURGED statement: %s", name, spec.Actions[name].Exec)
		}
	}
	dump := "a\nSET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ 'u1:1-5,\nu2:1-3';\nb\nSET @@GLOBAL.GTID_PURGED='x';\nc\n"
	for _, name := range []string{"import", "import_all"} {
		filter, _, ok := strings.Cut(spec.Actions[name].Exec, " | ")
		if !ok {
			t.Fatalf("%s pipes no filter in front of the client: %s", name, spec.Actions[name].Exec)
		}
		cmd := exec.Command("sh", "-c", filter)
		cmd.Stdin = strings.NewReader(dump)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s filter: %v", name, err)
		}
		if string(out) != "a\nb\nc\n" {
			t.Errorf("%s filter left %q", name, out)
		}
	}
}

// MySQL 9 refuses any statement touching its system schema, and a dump of
// --all-databases with --add-drop-database opens that schema's section with
// DROP DATABASE `mysql`, so a service-wide snapshot could not be restored. New
// dumps name the user databases instead, and the import drops the system
// schema's section from dumps taken before that.
func TestMysqlServiceWideSnapshotSkipsTheSystemSchema(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("loading the mysql preset: %v", err)
	}
	spec := p.Introspect.DatabasesEntity()
	export := spec.Actions["export_all"].Exec
	if strings.Contains(export, "--all-databases") {
		t.Errorf("export_all still dumps the system schemas: %s", export)
	}
	for _, schema := range []string{"'mysql'", "'information_schema'", "'performance_schema'", "'sys'"} {
		if !strings.Contains(export, schema) {
			t.Errorf("export_all does not leave %s out: %s", schema, export)
		}
	}

	dump := "-- Current Database: `app`\nA\n-- Current Database: `mysql`\nDROP DATABASE IF EXISTS `mysql`;\nM\n-- Current Database: `other`\nO\n"
	filter, _, _ := strings.Cut(spec.Actions["import_all"].Exec, " | ")
	cmd := exec.Command("sh", "-c", filter)
	cmd.Stdin = strings.NewReader(dump)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("import_all filter: %v", err)
	}
	want := "-- Current Database: `app`\nA\n-- Current Database: `other`\nO\n"
	if string(out) != want {
		t.Errorf("import_all filter left %q, want %q", out, want)
	}
}

// A service-wide snapshot of a server that cannot be asked for its databases
// has to fail, not store an empty dump as a good one; only a server that
// really holds no databases of its own may dump nothing.
func TestMysqlServiceWideExportFailsWhenTheServerCannotBeAsked(t *testing.T) {
	p, err := LoadPreset("mysql")
	if err != nil {
		t.Fatalf("loading the mysql preset: %v", err)
	}
	export := p.Introspect.DatabasesEntity().Actions["export_all"].Exec

	cases := []struct {
		name     string
		mysql    string
		wantErr  bool
		wantDump string
	}{
		{"server unreachable", "echo 'ERROR 2003' >&2; exit 1", true, ""},
		{"no databases of its own", "exit 0", false, ""},
		{"user databases", "printf 'app\\nother\\n'", false, "--databases app other"},
	}
	for _, c := range cases {
		bin := t.TempDir()
		writeScript(t, filepath.Join(bin, "mysql"), c.mysql)
		writeScript(t, filepath.Join(bin, "mysqldump"), `echo "$*"`)
		cmd := exec.Command("sh", "-c", export)
		cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin"}
		out, err := cmd.Output()
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, want error %v", c.name, err, c.wantErr)
		}
		if !strings.Contains(string(out), c.wantDump) || (c.wantDump == "" && len(out) != 0) {
			t.Errorf("%s: dump = %q, want %q", c.name, out, c.wantDump)
		}
	}
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
