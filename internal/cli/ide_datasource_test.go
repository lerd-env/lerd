package cli

import (
	"strings"
	"testing"
)

// The env file names the container, which is right inside the network and
// unusable from the machine, so the IDE has to be given the published port and
// loopback instead.
func TestIDEDataSourceUsesHostFacingCoordinates(t *testing.T) {
	env := &dbEnv{service: "postgres", connection: "pgsql", database: "shop", username: "postgres", password: "lerd"}
	ds, ok := ideDataSourceFor(env)
	if !ok {
		t.Fatal("postgres should have a JDBC dialect")
	}
	if strings.Contains(ds.URL, "lerd-postgres") {
		t.Errorf("URL points at the container: %q", ds.URL)
	}
	for _, want := range []string{"jdbc:postgresql://127.0.0.1:", "/shop"} {
		if !strings.Contains(ds.URL, want) {
			t.Errorf("URL missing %q: %q", want, ds.URL)
		}
	}
	// The IDE authenticates from its own sidecar, so the user rides there rather
	// than in the URL.
	if ds.User != "postgres" {
		t.Errorf("user = %q", ds.User)
	}
	if ds.Name != "shop (lerd)" {
		t.Errorf("name = %q", ds.Name)
	}
	if ds.Class != "org.postgresql.Driver" {
		t.Errorf("driver class = %q", ds.Class)
	}
}

func TestIDEDataSourceDialectPerFamily(t *testing.T) {
	cases := map[string]string{"mysql": "com.mysql.cj.jdbc.Driver", "postgres": "org.postgresql.Driver"}
	for service, class := range cases {
		ds, ok := ideDataSourceFor(&dbEnv{service: service, database: "shop", username: "root", password: "lerd"})
		if !ok {
			t.Fatalf("%s should have a dialect", service)
		}
		if ds.Class != class {
			t.Errorf("%s driver = %q, want %q", service, ds.Class, class)
		}
	}
}

// An engine lerd has no JDBC dialect for is left alone rather than written as a
// connection the IDE cannot open.
func TestIDEDataSourceSkipsEnginesWithNoDialect(t *testing.T) {
	if _, ok := ideDataSourceFor(&dbEnv{service: "mongo", database: "shop"}); ok {
		t.Error("mongo should not produce a JDBC data source")
	}
}
