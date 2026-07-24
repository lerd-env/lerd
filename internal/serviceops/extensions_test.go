package serviceops

import (
	"errors"
	"strings"
	"testing"
)

// The engine declares what it can offer, so lerd never carries a list of
// extension names in Go. The bundled postgres preset runs the PostGIS image and
// says so.
func TestDeclaredExtensionsComeFromThePreset(t *testing.T) {
	exts := DeclaredExtensions("postgres")
	if len(exts) == 0 {
		t.Fatal("the bundled postgres preset should declare what its image ships")
	}
	var postgis *Extension
	for i, e := range exts {
		if e.Name == "postgis" {
			postgis = &exts[i]
		}
	}
	if postgis == nil {
		t.Fatalf("postgis not declared: %+v", exts)
	}
	if len(postgis.Types) == 0 {
		t.Error("an extension with no types can never be matched to a dump that needs it")
	}
}

func TestDeclaredExtensionsEmptyForEnginesWithNone(t *testing.T) {
	if got := DeclaredExtensions("mysql"); len(got) != 0 {
		t.Errorf("mysql declared %+v", got)
	}
	if got := DeclaredExtensions("nosuchengine"); len(got) != 0 {
		t.Errorf("unknown engine declared %+v", got)
	}
}

// A dump reaches for an extension by naming one of its types in a column
// definition. Matching is on the type name, whether or not it is schema
// qualified, and never on a word that merely contains it.
func TestExtensionForLine(t *testing.T) {
	exts := []Extension{
		{Name: "vector", Types: []string{"vector", "halfvec"}},
		{Name: "postgis", Types: []string{"geometry", "geography"}},
	}
	cases := []struct {
		line string
		want string
	}{
		{"    embedding public.vector(1536),", "vector"},
		{"    embedding vector(1536),", "vector"},
		{"    shape public.geometry(Point,4326),", "postgis"},
		{"    half public.halfvec(4),", "vector"},
		{"CREATE TABLE public.users (id integer);", ""},
		{"INSERT INTO t VALUES ('a vector of values');", ""},
		{"    name text,", ""},
		// A column whose name embeds a type name is not a reference to it.
		{"    vectorized boolean,", ""},
		{"    geometry_id integer,", ""},
	}
	for _, c := range cases {
		got := ""
		if e := extensionForLine(exts, c.line); e != nil {
			got = e.Name
		}
		if got != c.want {
			t.Errorf("extensionForLine(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

// An extension with no distinctive type cannot be matched against a dump, so it
// says so instead, and lerd creates it wherever it creates a database.
func TestAlwaysExtensionsAreTheOnesCreatedUpFront(t *testing.T) {
	exts := []Extension{
		{Name: "postgis", Types: []string{"geometry"}},
		{Name: "timescaledb", Always: true},
		{Name: "vector", Types: []string{"vector"}, Always: true},
	}
	got := alwaysExtensions(exts)
	if len(got) != 2 || got[0].Name != "timescaledb" || got[1].Name != "vector" {
		t.Fatalf("always = %+v", got)
	}
}

// Every path that creates a database goes through here, so db:create, a fresh
// import and a snapshot restore all leave the engine's extensions in place.
func TestEnsureAlwaysExtensionsCreatesEachDeclaredOne(t *testing.T) {
	var calls []string
	defer stubExtensionCreate(func(service, database, name string) error {
		calls = append(calls, service+"/"+database+"/"+name)
		return nil
	})()
	exts := []Extension{{Name: "timescaledb", Always: true}, {Name: "postgis", Types: []string{"geometry"}}}
	if err := ensureAlwaysExtensions("postgres-timescaledb", "shop", exts); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "postgres-timescaledb/shop/timescaledb" {
		t.Fatalf("calls = %v", calls)
	}
}

// A database that cannot get the extension its engine promises is not the
// database it claims to be, so the failure is not swallowed.
func TestEnsureAlwaysExtensionsSurfacesAFailure(t *testing.T) {
	defer stubExtensionCreate(func(_, _, _ string) error { return errors.New("no such extension") })()
	err := ensureAlwaysExtensions("pg", "shop", []Extension{{Name: "timescaledb", Always: true}})
	if err == nil || !strings.Contains(err.Error(), "timescaledb") {
		t.Fatalf("err = %v", err)
	}
}

// A site created before its engine declared an extension has to pick it up, or
// the declaration only ever reaches databases made after the upgrade.
func TestEnsureExtensionsAppliesToAnExistingDatabase(t *testing.T) {
	var calls int
	defer stubExtensionCreate(func(_, _, _ string) error { calls++; return nil })()
	if err := ensureAlwaysExtensions("pg", "already_there", []Extension{{Name: "vector", Always: true}}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// A database is dropped while the site's own pool still holds it open, and that
// pool reconnects the instant it is closed, so terminating and then dropping
// races. The single statement that does both is what the drop has to use.
func TestPostgresDropIsAtomicAgainstReconnects(t *testing.T) {
	sql := postgresDropSQL("shop")
	if !strings.Contains(sql, "WITH (FORCE)") {
		t.Errorf("drop is not atomic: %q", sql)
	}
	if !strings.Contains(sql, `"shop"`) {
		t.Errorf("database name not quoted as an identifier: %q", sql)
	}
	if !strings.Contains(sql, "IF EXISTS") {
		t.Errorf("dropping an absent database must not be an error: %q", sql)
	}
	if got := postgresDropSQL(`we"ird`); !strings.Contains(got, `"we""ird"`) {
		t.Errorf("quote in a name not escaped: %q", got)
	}
}
