package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectDB_HostSocketPath(t *testing.T) {
	if got := (ProjectDB{}).HostSocketPath(); got != DefaultHostMySQLSocket {
		t.Fatalf("default HostSocketPath = %q, want %q", got, DefaultHostMySQLSocket)
	}
	custom := "/var/run/mysqld/mysqld.sock"
	if got := (ProjectDB{Socket: custom}).HostSocketPath(); got != custom {
		t.Fatalf("custom HostSocketPath = %q, want %q", got, custom)
	}
}

func TestIsEmpty_HostDBExternal(t *testing.T) {
	if !(&ProjectConfig{}).IsEmpty() {
		t.Fatalf("zero ProjectConfig should be empty")
	}
	if (&ProjectConfig{DB: ProjectDB{External: true}}).IsEmpty() {
		t.Fatalf("config carrying only db.external should NOT be empty (else SaveProjectConfig-on-empty paths would drop it)")
	}
	if (&ProjectConfig{DB: ProjectDB{Socket: "/run/mysqld/mysqld.sock"}}).IsEmpty() {
		t.Fatalf("config carrying only db.socket should NOT be empty")
	}
}

func TestGlobalConfig_DefaultDBBackend(t *testing.T) {
	// Nil receiver and the zero value both mean container (lerd's own MySQL).
	if got := (*GlobalConfig)(nil).DefaultDBBackend(); got != DBBackendContainer {
		t.Errorf("nil receiver = %q, want %q", got, DBBackendContainer)
	}
	if got := (&GlobalConfig{}).DefaultDBBackend(); got != DBBackendContainer {
		t.Errorf("zero value = %q, want %q", got, DBBackendContainer)
	}
	host := &GlobalConfig{}
	host.Database.DefaultBackend = DBBackendHost
	if got := host.DefaultDBBackend(); got != DBBackendHost {
		t.Errorf("host configured = %q, want %q", got, DBBackendHost)
	}
	// An unrecognised value normalises back to container rather than leaking out.
	junk := &GlobalConfig{}
	junk.Database.DefaultBackend = "nonsense"
	if got := junk.DefaultDBBackend(); got != DBBackendContainer {
		t.Errorf("junk value = %q, want %q", got, DBBackendContainer)
	}
}

func TestProjectConfig_IsMySQLFamilyDB(t *testing.T) {
	// Nil and unspecified DB both assume MySQL family (db.external is the
	// host-MySQL feature; an unset DB shouldn't block enabling it).
	if !(*ProjectConfig)(nil).IsMySQLFamilyDB() {
		t.Error("nil receiver should be MySQL family (assumed)")
	}
	if !(&ProjectConfig{}).IsMySQLFamilyDB() {
		t.Error("empty config should be MySQL family (assumed)")
	}
	// db.service wins over the services list.
	if !(&ProjectConfig{DB: ProjectDB{Service: "mariadb"}}).IsMySQLFamilyDB() {
		t.Error("db.service=mariadb should be MySQL family")
	}
	if (&ProjectConfig{DB: ProjectDB{Service: "postgres"}}).IsMySQLFamilyDB() {
		t.Error("db.service=postgres should NOT be MySQL family")
	}
	// Falls back to the services list when db.service is unset.
	if !(&ProjectConfig{Services: []ProjectService{{Name: "mysql"}}}).IsMySQLFamilyDB() {
		t.Error("services=[mysql] should be MySQL family")
	}
	if (&ProjectConfig{Services: []ProjectService{{Name: "postgres"}}}).IsMySQLFamilyDB() {
		t.Error("services=[postgres] should NOT be MySQL family")
	}
}

func TestSetProjectDBExternal_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Enable with the default socket.
	if err := SetProjectDBExternal(dir, true, ""); err != nil {
		t.Fatalf("enable: %v", err)
	}
	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load after enable: %v", err)
	}
	if !cfg.DB.External {
		t.Fatalf("External = false after enable, want true")
	}
	if cfg.DB.Socket != "" {
		t.Fatalf("Socket = %q after default enable, want empty (falls back to default)", cfg.DB.Socket)
	}
	if got := cfg.DB.HostSocketPath(); got != DefaultHostMySQLSocket {
		t.Fatalf("HostSocketPath = %q, want default %q", got, DefaultHostMySQLSocket)
	}

	// Persisted file carries the committed marker.
	raw, err := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if err != nil {
		t.Fatalf("read .lerd.yaml: %v", err)
	}
	if !strings.Contains(string(raw), "external: true") {
		t.Fatalf(".lerd.yaml missing the external marker:\n%s", raw)
	}

	// Custom socket overrides the default.
	custom := "/tmp/mysqld.sock"
	if err := SetProjectDBExternal(dir, true, custom); err != nil {
		t.Fatalf("enable custom: %v", err)
	}
	cfg, _ = LoadProjectConfig(dir)
	if cfg.DB.Socket != custom {
		t.Fatalf("Socket = %q, want %q", cfg.DB.Socket, custom)
	}
	if got := cfg.DB.HostSocketPath(); got != custom {
		t.Fatalf("HostSocketPath = %q, want %q", got, custom)
	}

	// Disable clears both fields.
	if err := SetProjectDBExternal(dir, false, ""); err != nil {
		t.Fatalf("disable: %v", err)
	}
	cfg, _ = LoadProjectConfig(dir)
	if cfg.DB.External || cfg.DB.Socket != "" {
		t.Fatalf("after disable: External=%v Socket=%q, want false/empty", cfg.DB.External, cfg.DB.Socket)
	}
}
