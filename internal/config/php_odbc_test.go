package config

import "testing"

// unixODBC resolves a DSN's Driver={...} without regard to case, so lerd must
// too: a second registration under a differently-cased name is the same driver
// moving, not a new one, and two sections would make the registry ambiguous.
func TestSetODBCDriverReplacesByFoldedName(t *testing.T) {
	c := &GlobalConfig{}
	c.SetODBCDriver(ODBCDriver{Name: "HDBODBC", Driver: "/opt/old/libodbcHDB.so"})
	c.SetODBCDriver(ODBCDriver{Name: "hdbodbc", Driver: "/opt/new/libodbcHDB.so"})

	drivers := c.GetODBCDrivers()
	if len(drivers) != 1 {
		t.Fatalf("GetODBCDrivers() = %v, want one entry", drivers)
	}
	if drivers[0].Driver != "/opt/new/libodbcHDB.so" {
		t.Errorf("driver path = %q, want the re-registered one", drivers[0].Driver)
	}

	found, ok := c.FindODBCDriver("HdbOdbc")
	if !ok || found.Driver != "/opt/new/libodbcHDB.so" {
		t.Errorf("FindODBCDriver = (%+v, %v), want the registered driver", found, ok)
	}
}

func TestRemoveODBCDriver(t *testing.T) {
	c := &GlobalConfig{}
	c.SetODBCDriver(ODBCDriver{Name: "HDBODBC", Driver: "/opt/hana/libodbcHDB.so"})
	c.SetODBCDriver(ODBCDriver{Name: "Oracle", Driver: "/opt/oracle/libsqora.so"})

	if !c.RemoveODBCDriver("hdbodbc") {
		t.Error("RemoveODBCDriver reported nothing removed for a registered driver")
	}
	if c.RemoveODBCDriver("hdbodbc") {
		t.Error("RemoveODBCDriver reported a removal for a driver that was already gone")
	}
	if drivers := c.GetODBCDrivers(); len(drivers) != 1 || drivers[0].Name != "Oracle" {
		t.Errorf("GetODBCDrivers() = %v, want Oracle alone", drivers)
	}
}
