package dbview

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// An external server has no unit, so a running check on one would hide every
// database it holds without saying why.
func TestLoad_ListsAnExternalServersDatabases(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tools := t.TempDir()
	if err := os.WriteFile(filepath.Join(tools, "mysql"), []byte("#!/bin/sh\nprintf 'shop\\t2048\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &config.CustomService{
		Name:   "dbngin",
		Family: "mysql",
		External: &config.ExternalDB{
			Host: "host.containers.internal", Port: ln.Addr().(*net.TCPAddr).Port, User: "me", Tools: tools,
		},
		Introspect: &config.Introspect{ListDatabases: "mysql -sN"},
	}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatal(err)
	}

	eng := Load("dbngin", nil)

	if eng.Error != "" {
		t.Fatal(eng.Error)
	}
	if !eng.Running || len(eng.Databases) != 1 || eng.Databases[0].Name != "shop" {
		t.Fatalf("engine = %+v", eng)
	}

	ln.Close()
	if eng := Load("dbngin", nil); eng.Running {
		t.Error("a server that stopped answering is not running")
	}
}
