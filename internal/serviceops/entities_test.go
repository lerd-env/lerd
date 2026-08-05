package serviceops

import (
	"strings"
	"testing"
)

const entityEngineYAML = `name: myengine
image: example/engine:1
introspect:
  entities:
    - kind: buckets
      list: list-buckets
      columns:
        - key: objects
          format: number
        - key: size
          format: bytes
      actions:
        create: make-bucket {{name}}
        delete:
          exec: remove-bucket {{name}}
          destructive: true
`

func TestEntityForResolvesDeclaredKind(t *testing.T) {
	writeCustomService(t, "myengine", entityEngineYAML)
	spec := EntityFor("myengine", "buckets")
	if spec == nil {
		t.Fatal("declared buckets entity did not resolve")
	}
	if spec.List != "list-buckets" || len(spec.Columns) != 2 {
		t.Errorf("spec = %+v", spec)
	}
	if EntityFor("myengine", "queues") != nil {
		t.Error("undeclared kind must resolve to nil")
	}
}

func TestEntityForDatabasesFallsBackToLegacyField(t *testing.T) {
	writeCustomService(t, "myengine", "name: myengine\nimage: example/engine:1\nintrospect:\n  list_databases: echo hi\n")
	spec := EntityFor("myengine", "databases")
	if spec == nil || spec.List != "echo hi" {
		t.Fatalf("legacy list_databases must synthesize a databases entity, got %+v", spec)
	}
	if len(spec.Actions) != 0 {
		t.Errorf("synthesized spec must carry no actions: %+v", spec.Actions)
	}
}

func TestEntityForPrefersPresetOverBareStoredDefinition(t *testing.T) {
	// An engine installed before entities existed resolves them from the preset
	// it came from, the same way IntrospectCommand always has.
	writeCustomService(t, "mysql", "name: mysql\nimage: docker.io/library/mysql:8\npreset: mysql\n")
	spec := EntityFor("mysql", "databases")
	if spec == nil || spec.List == "" {
		t.Fatal("want the bundled mysql preset entity, got none")
	}
}

// The databases kind has its own tab, so the generic overview must not repeat
// it alongside the other kinds a service declares.
func TestServiceEntitiesExcludesDatabases(t *testing.T) {
	writeCustomService(t, "myengine", `name: myengine
image: example/engine:1
introspect:
  entities:
    - kind: databases
      list: list-dbs
    - kind: buckets
      list: list-buckets
`)
	kinds := EntityKinds("myengine")
	if len(kinds) != 1 || kinds[0] != "buckets" {
		t.Fatalf("kinds = %v, want [buckets]", kinds)
	}
	if EntityKinds("mysql") != nil {
		t.Error("an engine declaring only databases must expose no generic kinds")
	}
}

func TestExpandEntityCommandValidatesAndSubstitutes(t *testing.T) {
	cmd, err := expandEntityCommand("do-thing {{name}} on {{name}}", "shop_db")
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "do-thing shop_db on shop_db" {
		t.Errorf("expanded = %q", cmd)
	}
	// The name pattern is the injection guard: anything outside it never
	// reaches the shell.
	for _, bad := range []string{"", "a;rm -rf /", "x`y`", "a b", "$(x)", "-"} {
		if _, err := expandEntityCommand("do {{name}}", bad); err == nil {
			t.Errorf("name %q must be rejected", bad)
		}
	}
}

func TestParseEntityRows(t *testing.T) {
	rows := parseEntityRows([]byte("alpha\t3\t1024\nbeta\t0\t0\n\n"))
	if len(rows) != 2 {
		t.Fatalf("got %d rows: %+v", len(rows), rows)
	}
	if rows[0].Name != "alpha" || len(rows[0].Values) != 2 || rows[0].Values[1] != "1024" {
		t.Errorf("row 0 = %+v", rows[0])
	}
}

// An image-less entity execs inside the service container with the fixed
// admin credentials; one with a client image runs ephemerally on the lerd
// network with only its own env, and the image's entrypoint is overridden
// because client images make their tool the entrypoint.
func TestEntityCommandArgs(t *testing.T) {
	execArgs := entityCommandArgs("mysql", "", nil, "list-cmd", false)
	joined := strings.Join(execArgs, " ")
	if execArgs[0] != "exec" || !strings.Contains(joined, "lerd-mysql sh -c list-cmd") {
		t.Errorf("exec args = %v", execArgs)
	}
	if !strings.Contains(joined, "MYSQL_PWD=") {
		t.Errorf("exec args missing admin env: %v", execArgs)
	}

	runArgs := entityCommandArgs("rustfs", "docker.io/rclone/rclone:latest", []string{"A=b"}, "tar-cmd", true)
	joined = strings.Join(runArgs, " ")
	for _, want := range []string{"run --rm -i", "--network lerd", "--entrypoint sh", "-e A=b", "docker.io/rclone/rclone:latest -c tar-cmd"} {
		if !strings.Contains(joined, want) {
			t.Errorf("run args missing %q: %v", want, runArgs)
		}
	}
	if strings.Contains(joined, "MYSQL_PWD=") {
		t.Errorf("a client run must not carry the exec admin env: %v", runArgs)
	}
}

// An action can override its entity's client image, so one entity lists
// through one tool and streams archives through another.
func TestActionRuntimeOverride(t *testing.T) {
	writeCustomService(t, "myengine", `name: myengine
image: example/engine:1
introspect:
  entities:
    - kind: buckets
      list: list-cmd
      image: example/client:1
      env:
        - CLIENT=1
      actions:
        create: create-cmd {{name}}
        export:
          exec: export-cmd {{name}}
          filename: "{{name}}.tar"
          image: example/streamer:1
          env:
            - STREAMER=1
`)
	spec := EntityFor("myengine", "buckets")
	image, env := actionRuntime(spec, spec.Actions["create"])
	if image != "example/client:1" || len(env) != 1 || env[0] != "CLIENT=1" {
		t.Errorf("create runtime = %q %v", image, env)
	}
	image, env = actionRuntime(spec, spec.Actions["export"])
	if image != "example/streamer:1" || len(env) != 1 || env[0] != "STREAMER=1" {
		t.Errorf("export runtime = %q %v", image, env)
	}
	if got := EntityExportFilename("myengine", "buckets", "photos"); got != "photos.tar" {
		t.Errorf("filename = %q", got)
	}
	if got := EntityExportFilename("myengine", "nokind", "photos"); got != "photos.dump" {
		t.Errorf("fallback filename = %q", got)
	}
	// The fallback becomes an output path in the CLI and MCP export paths, so a
	// name that never passed validation must not shape it.
	if got := EntityExportFilename("myengine", "nokind", "../../../etc/passwd"); strings.Contains(got, "/") {
		t.Errorf("fallback filename carries a path: %q", got)
	}
}

// A snapshot stores the same dump an export produces, gzipped in the container
// so the bytes cross podman exec compressed.
func TestEntitySnapshotCommandsWrapDeclaredActions(t *testing.T) {
	dump := entitySnapshotDumpCommand("export-cmd shop")
	if !strings.Contains(dump, "export-cmd shop") || !strings.Contains(dump, "gzip -c") {
		t.Errorf("dump = %q", dump)
	}
	restore := entitySnapshotRestoreCommand("import-cmd shop")
	if !strings.HasPrefix(restore, "gunzip -c") || !strings.Contains(restore, "import-cmd shop") {
		t.Errorf("restore = %q", restore)
	}
}

// A snapshot downloads in the engine's own dump format, so its filename must
// take the extension from the declared export, not a hardcoded .sql: a mongo
// snapshot is a mongodump archive, and saving it as db.sql mislabels it.
func TestSnapshotExportFilename(t *testing.T) {
	writeCustomService(t, "mongo", `name: mongo
image: docker.io/library/mongo:7
introspect:
  entities:
    - kind: databases
      list: echo x
      actions:
        export:
          exec: mongodump --archive
          filename: "{{name}}.archive"
`)
	if got := SnapshotExportFilename("mongo", "before-mutation-20260101-000000"); got != "before-mutation-20260101-000000.archive" {
		t.Errorf("mongo snapshot filename = %q, want .archive", got)
	}
	// A bundled SQL engine keeps .sql from its declared filename.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := SnapshotExportFilename("postgres", "nightly"); got != "nightly.sql" {
		t.Errorf("postgres snapshot filename = %q, want nightly.sql", got)
	}
	// An engine that declares no export filename falls back to a neutral, safe name.
	if got := SnapshotExportFilename("mysql", "../etc/passwd"); strings.Contains(got, "/") {
		t.Errorf("snapshot filename carries a path: %q", got)
	}
}

func TestDeclaresDatabasesFollowsTheDeclaration(t *testing.T) {
	writeCustomService(t, "myengine", entityEngineYAML)
	if DeclaresDatabases("myengine") {
		t.Error("an engine declaring only buckets must not count as a database engine")
	}
	writeCustomService(t, "analytics", `name: analytics
image: example/analytics:1
introspect:
  entities:
    - kind: databases
      list: list-dbs
`)
	if !DeclaresDatabases("analytics") {
		t.Error("a declared databases entity must count whatever the family is")
	}
	writeCustomService(t, "legacy", "name: legacy\nimage: example/legacy:1\nintrospect:\n  list_databases: echo hi\n")
	if !DeclaresDatabases("legacy") {
		t.Error("the legacy list_databases field must count too")
	}
}

// A dump that fails must not be stored as a snapshot. The command pipes into
// gzip, so without pipefail the shell reports gzip's status and a failed dump
// looks like a clean one, leaving an empty archive that restores nothing.
func TestEntitySnapshotDumpCommandFailsOnADeadDump(t *testing.T) {
	dump := entitySnapshotDumpCommand("export-cmd shop")
	if !strings.Contains(dump, "pipefail") {
		t.Errorf("dump command lets a failed dump exit 0: %q", dump)
	}
}
