package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The registry is generated, so a driver registered once has to come out as a
// section unixODBC resolves Driver={...} through.
func TestRenderOdbcInst(t *testing.T) {
	out := RenderOdbcInst([]config.ODBCDriver{
		{Name: "HDBODBC", Driver: "/opt/hana/libodbcHDB.so", Description: "SAP HANA"},
		{Name: "Oracle", Driver: "/opt/oracle/libsqora.so"},
		{Name: "bad;name", Driver: "/opt/evil.so"},
	})
	for _, want := range []string{
		"[HDBODBC]\nDescription = SAP HANA\nDriver = /opt/hana/libodbcHDB.so\n",
		"[Oracle]\nDriver = /opt/oracle/libsqora.so\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered registry missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "bad;name") {
		t.Errorf("a name that cannot be a section was written anyway:\n%s", out)
	}
}

// An empty registry still has to be a file: podman puts a directory at a
// bind-mount source that is not there, and /etc/odbcinst.ini as a directory
// leaves the driver manager reading nothing.
func TestRenderOdbcInstIsNeverEmpty(t *testing.T) {
	if out := RenderOdbcInst(nil); strings.TrimSpace(out) == "" {
		t.Error("registry with no drivers rendered empty, the mount source must stay a file")
	}
}

func TestEnsureOdbcInstReplacesAStaleDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := os.MkdirAll(config.OdbcInstFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOdbcInst(); err != nil {
		t.Fatalf("EnsureOdbcInst: %v", err)
	}
	info, err := os.Stat(config.OdbcInstFile())
	if err != nil || info.IsDir() {
		t.Fatalf("odbcinst.ini is not a regular file after Ensure: %v", err)
	}
}

// Every PHP container has to mount the registry, or a driver registered on one
// runtime is invisible on another.
func TestFPMQuadletMountsTheDriverRegistry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(tmp, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	want := "Volume=" + config.OdbcInstFile() + ":/etc/odbcinst.ini:ro"
	if !strings.Contains(content, want) {
		t.Errorf("FPM quadlet does not mount the ODBC driver registry (%s):\n%s", want, content)
	}
}

// A site whose own image writes /etc/odbcinst.ini already has its drivers. An
// empty registry mounted over that file would take every one of them away, so
// nothing is mounted until a driver is registered with lerd.
func TestFPMQuadletLeavesAnImageRegistryAloneWhenNothingIsRegistered(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if strings.Contains(content, "Volume="+config.OdbcInstFile()) {
		t.Errorf("quadlet mounts an empty ODBC registry over the image's own:\n%s", content)
	}
}

// A driver directory that is gone would make podman refuse to start the
// container, which is a worse failure than the driver simply not loading.
func TestODBCDriverDirsSkipsWhatIsNotThere(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	present := filepath.Join(tmp, "drivers")
	if err := os.MkdirAll(present, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "Here", Driver: filepath.Join(present, "libodbc.so")})
		c.SetODBCDriver(config.ODBCDriver{Name: "Gone", Driver: filepath.Join(tmp, "missing", "libodbc.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}
	dirs := ODBCDriverDirs()
	if len(dirs) != 1 || dirs[0] != present {
		t.Errorf("ODBCDriverDirs() = %v, want only %q", dirs, present)
	}
}

// A FrankenPHP site mounts its project, not the whole home, so both the registry
// and the directory the driver lives in have to travel with it.
func TestFrankenPHPQuadletCarriesTheDriverAndRegistry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	driverDir := filepath.Join(tmp, "hdbclient")
	if err := os.MkdirAll(driverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := GenerateFrankenPHPQuadlet("myapp", filepath.Join(tmp, "myapp"), "8.4", nil, nil)
	if err != nil {
		t.Fatalf("GenerateFrankenPHPQuadlet: %v", err)
	}
	for _, want := range []string{
		"Volume=" + config.OdbcInstFile() + ":/etc/odbcinst.ini:ro",
		"Volume=" + driverDir + ":" + driverDir + ":ro",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("FrankenPHP quadlet missing %q:\n%s", want, content)
		}
	}
}

// Real `ldd` output for the SAP HANA driver in the lerd image: every library
// resolves, so a library-only check calls this driver fine, and unixODBC then
// answers "file not found" for a driver it could not dlopen. The missing glibc
// symbols are the actual reason and the only useful thing to report.
const hanaProbeOutput = `LERD_FOUND
[HDBODBC]
	/lib/ld-musl-aarch64.so.1 (0xffff92e00000)
	libstdc++.so.6 => /usr/lib/libstdc++.so.6 (0xffff91f3b000)
	libc.so.6 => /lib/ld-musl-aarch64.so.1 (0xffff92e00000)
	ld-linux-aarch64.so.1 => /lib/ld-linux-aarch64.so.1 (0xffff92dae000)
Error relocating /opt/hana/libodbcHDB.so: qfcvt_r: symbol not found
Error relocating /opt/hana/libodbcHDB.so: qecvt_r: symbol not found
Error relocating /opt/hana/libodbcHDB.so: backtrace: symbol not found
`

func TestParseODBCProbeCatchesUnresolvedSymbols(t *testing.T) {
	status := parseODBCProbe(hanaProbeOutput, "HDBODBC")
	if !status.Found || !status.Registered {
		t.Fatalf("probe = %+v, want the driver found and registered", status)
	}
	if len(status.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want none: every library did resolve", status.Unresolved)
	}
	want := "qfcvt_r,qecvt_r,backtrace"
	if got := strings.Join(status.Symbols, ","); got != want {
		t.Errorf("Symbols = %q, want %q", got, want)
	}
	if status.Loadable() {
		t.Error("a driver with unresolved symbols reported as loadable, which is the false pass this check exists to prevent")
	}
}

func TestParseODBCProbeAcceptsADriverThatLoads(t *testing.T) {
	out := "LERD_FOUND\n[PostgreSQL]\n\t/lib/ld-musl-aarch64.so.1 (0xffff92e00000)\n\tlibodbc.so.2 => /usr/lib/libodbc.so.2 (0xffff91f3b000)\n"
	status := parseODBCProbe(out, "PostgreSQL")
	if !status.Loadable() {
		t.Errorf("probe = %+v, want a clean driver to read as loadable", status)
	}
}

func TestParseODBCProbeReportsAMissingLibrary(t *testing.T) {
	out := "LERD_FOUND\n[Oracle]\nError loading shared library libaio.so.1: No such file or directory\n"
	status := parseODBCProbe(out, "Oracle")
	if len(status.Unresolved) != 1 || status.Unresolved[0] != "libaio.so.1" {
		t.Errorf("Unresolved = %v, want [libaio.so.1]", status.Unresolved)
	}
	if status.Loadable() {
		t.Error("a driver missing a library reported as loadable")
	}
}
