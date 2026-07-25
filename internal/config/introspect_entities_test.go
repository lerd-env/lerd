package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

const entitiesYAML = `
name: engine
image: example/engine:1
introspect:
  entities:
    - kind: databases
      format: sql
      list: list-cmd
      columns:
        - key: size
          label: Size
          format: bytes
      actions:
        create: create-cmd {{name}}
        drop:
          exec: drop-cmd {{name}}
          destructive: true
        export:
          exec: export-cmd {{name}}
          filename: "{{name}}.sql"
        import: import-cmd {{name}}
`

func TestEntitySpecParsesActionsAndColumns(t *testing.T) {
	var svc CustomService
	if err := yaml.Unmarshal([]byte(entitiesYAML), &svc); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	spec := svc.Introspect.Entity("databases")
	if spec == nil {
		t.Fatal("no databases entity parsed")
	}
	if spec.List != "list-cmd" || spec.Format != "sql" {
		t.Errorf("spec = %+v", spec)
	}
	if len(spec.Columns) != 1 || spec.Columns[0].Key != "size" || spec.Columns[0].Format != "bytes" {
		t.Errorf("columns = %+v", spec.Columns)
	}
	// A bare string is shorthand for an action with just an exec.
	if got := spec.Actions["create"].Exec; got != "create-cmd {{name}}" {
		t.Errorf("create exec = %q", got)
	}
	if spec.Actions["create"].Destructive {
		t.Error("create must not be destructive")
	}
	drop := spec.Actions["drop"]
	if drop.Exec != "drop-cmd {{name}}" || !drop.Destructive {
		t.Errorf("drop = %+v", drop)
	}
	if got := spec.Actions["export"].Filename; got != "{{name}}.sql" {
		t.Errorf("export filename = %q", got)
	}
}

func TestEntityLookupMissesUnknownKind(t *testing.T) {
	var svc CustomService
	if err := yaml.Unmarshal([]byte(entitiesYAML), &svc); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if svc.Introspect.Entity("buckets") != nil {
		t.Error("unknown kind must resolve to nil")
	}
	var nilIntrospect *Introspect
	if nilIntrospect.Entity("databases") != nil {
		t.Error("nil introspect must resolve to nil")
	}
}

// A store preset published before entities existed only declares list_databases.
// It must keep listing through the same entity surface, as a list-only spec.
func TestDatabasesEntitySynthesizedFromLegacyField(t *testing.T) {
	legacy := &Introspect{ListDatabases: "legacy-list"}
	spec := legacy.DatabasesEntity()
	if spec == nil {
		t.Fatal("legacy list_databases produced no databases entity")
	}
	if spec.List != "legacy-list" || spec.Kind != "databases" {
		t.Errorf("spec = %+v", spec)
	}
	if len(spec.Actions) != 0 {
		t.Errorf("a synthesized spec must declare no actions, got %+v", spec.Actions)
	}
}

// An action can run on different tooling than the rest of its entity: bucket
// archives need tar, which the listing client's image does not carry.
func TestEntityActionImageOverride(t *testing.T) {
	const y = `
name: engine
image: example/engine:1
introspect:
  entities:
    - kind: buckets
      list: list-cmd
      image: example/client:1
      env:
        - CLIENT_HOST=engine
      actions:
        create: create-cmd {{name}}
        export:
          exec: export-cmd {{name}}
          image: example/streamer:1
          env:
            - STREAMER_HOST=engine
`
	var svc CustomService
	if err := yaml.Unmarshal([]byte(y), &svc); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	spec := svc.Introspect.Entity("buckets")
	if spec.Image != "example/client:1" || len(spec.Env) != 1 {
		t.Errorf("spec image/env = %q %v", spec.Image, spec.Env)
	}
	exp := spec.Actions["export"]
	if exp.Image != "example/streamer:1" || len(exp.Env) != 1 || exp.Env[0] != "STREAMER_HOST=engine" {
		t.Errorf("export override = %+v", exp)
	}
	if spec.Actions["create"].Image != "" {
		t.Error("an action without an override must not carry one")
	}
}

func TestDatabasesEntityPrefersDeclaredEntities(t *testing.T) {
	var svc CustomService
	if err := yaml.Unmarshal([]byte(entitiesYAML), &svc); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	svc.Introspect.ListDatabases = "legacy-list"
	spec := svc.Introspect.DatabasesEntity()
	if spec == nil || spec.List != "list-cmd" {
		t.Fatalf("declared entity must win over the legacy field, got %+v", spec)
	}
	if (&Introspect{}).DatabasesEntity() != nil {
		t.Error("an introspect block with neither form must resolve to nil")
	}
}
