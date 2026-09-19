package podman

import (
	"github.com/geodro/lerd/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// A vendor driver is a licensed file the container only ever reads, and the
// FrankenPHP quadlet already mounts its directory read-only. The FPM quadlet
// reached the same directory through ExtraVolumePaths, which exists for parked
// projects and mounts read-write, so the two runtimes disagreed on the same path.
func TestFPMQuadletMountsDriverDirsReadOnly(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	driverDir := t.TempDir() // outside the fake home, so %h:%h cannot cover it
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if want := "Volume=" + driverDir + ":" + driverDir + ":ro"; !strings.Contains(content, want) {
		t.Errorf("FPM quadlet does not mount the driver directory read-only (%s):\n%s", want, content)
	}
	if bad := "Volume=" + driverDir + ":" + driverDir + ":rw"; strings.Contains(content, bad) {
		t.Errorf("driver directory is still mounted read-write:\n%s", content)
	}
}

// ExtraVolumePaths is the parked-project and site-path list; a driver directory
// riding along in it is what made the mount read-write. It also feeds the nginx
// quadlet, which has no business reaching a database driver at all.
func TestExtraVolumePathsLeavesDriverDirsAlone(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	driverDir := t.TempDir()
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}
	for _, p := range ExtraVolumePaths() {
		if p == driverDir {
			t.Errorf("ExtraVolumePaths() still carries the ODBC driver dir %q", p)
		}
	}
}

// A driver that lives inside a parked directory keeps that directory's
// read-write mount: the parked project is the reason the path is mounted at all,
// and a read-only line for the same path would take the write access away.
func TestFPMQuadletKeepsAParkedDirWritable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	parked := t.TempDir()
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.ParkedDirectories = []string{parked}
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(parked, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if want := "Volume=" + parked + ":" + parked + ":rw"; !strings.Contains(content, want) {
		t.Errorf("parked directory lost its read-write mount (%s):\n%s", want, content)
	}
	if bad := "Volume=" + parked + ":" + parked + ":ro"; strings.Contains(content, bad) {
		t.Errorf("parked directory was downgraded to read-only:\n%s", content)
	}
}

// A driver under $HOME is already reachable through the quadlet's %h:%h line,
// which is read-write. Adding a read-only line for the same subtree would make
// the directory holding the driver unwritable for PHP, which is a regression for
// any project that keeps its driver inside the tree it also writes to.
func TestFPMQuadletLeavesDriverDirsUnderHomeToTheHomeMount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	driverDir := filepath.Join(home, "vendordrv")
	if err := os.MkdirAll(driverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if strings.Contains(content, "Volume="+driverDir+":") {
		t.Errorf("driver dir under $HOME got its own mount, shadowing the read-write %%h:%%h one:\n%s", content)
	}
	if want := "Volume=" + config.OdbcInstFile() + ":/etc/odbcinst.ini:ro"; !strings.Contains(content, want) {
		t.Errorf("registry mount missing (%s):\n%s", want, content)
	}
}

// The FPM quadlet skips a driver dir under $HOME because %h:%h already covers
// it; a FrankenPHP site mounts its project and not the home, so the same
// directory has to travel with it or the driver is a path it cannot see. The two
// runtimes disagree here on purpose.
func TestFrankenPHPQuadletStillCarriesADriverUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	driverDir := filepath.Join(home, "vendordrv")
	if err := os.MkdirAll(driverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := GenerateFrankenPHPQuadlet("myapp", filepath.Join(home, "myapp"), "8.4", nil, nil)
	if err != nil {
		t.Fatalf("GenerateFrankenPHPQuadlet: %v", err)
	}
	if want := "Volume=" + driverDir + ":" + driverDir + ":ro"; !strings.Contains(content, want) {
		t.Errorf("FrankenPHP quadlet dropped a driver dir under $HOME (%s):\n%s", want, content)
	}
}

// A driver kept inside a parked project is reached through that project's
// read-write mount. A read-only line for the subdirectory would nest inside it
// and take write access away from exactly the tree the user parked to work in.
func TestFPMQuadletLeavesADriverDirInsideAParkedDirAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	parked := t.TempDir()
	driverDir := filepath.Join(parked, "vendor", "odbc")
	if err := os.MkdirAll(driverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.ParkedDirectories = []string{parked}
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if strings.Contains(content, "Volume="+driverDir+":") {
		t.Errorf("driver dir inside a parked dir got its own mount, nesting read-only inside the parked read-write one:\n%s", content)
	}
	if want := "Volume=" + parked + ":" + parked + ":rw"; !strings.Contains(content, want) {
		t.Errorf("parked dir lost its read-write mount (%s):\n%s", want, content)
	}
}

// The other direction: a driver sitting in a directory that happens to contain a
// parked project. Both have to be mounted, and the driver's directory has to come
// first, or the parked project's mount is the one that disappears underneath it.
// Leaving the driver dir out instead is not an option: the probe mounts it
// directly and would report a driver the FPM container cannot actually see as
// one that loads.
func TestFPMQuadletMountsADriverDirBeforeAParkedDirInsideIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	driverDir := t.TempDir()
	parked := filepath.Join(driverDir, "project")
	if err := os.MkdirAll(parked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.ParkedDirectories = []string{parked}
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: filepath.Join(driverDir, "libodbcHDB.so")})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	driverLine := "Volume=" + driverDir + ":" + driverDir + ":ro"
	parkedLine := "Volume=" + parked + ":" + parked + ":rw"
	di, pi := strings.Index(content, driverLine), strings.Index(content, parkedLine)
	if di < 0 {
		t.Fatalf("driver dir is not mounted (%s), so the container cannot see the driver:\n%s", driverLine, content)
	}
	if pi < 0 {
		t.Fatalf("parked dir lost its mount (%s):\n%s", parkedLine, content)
	}
	if di > pi {
		t.Errorf("driver dir is mounted after the parked dir inside it, which hides the parked mount:\n%s", content)
	}
}

// A vendor package installs its ODBC driver straight into a system library
// directory, so following the vendor's own instructions points lerd at one. The
// container keeps its own libraries there, and mounting the host's over them
// leaves PHP with nothing to link against: the unit restart-loops and every site
// on that version answers 502. Nothing may mount such a directory.
func TestODBCDriverDirsRefusesTheContainersOwnRuntime(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "UsrLib", Driver: "/usr/lib/psqlodbcw.so"})
		c.SetODBCDriver(config.ODBCDriver{Name: "Lib", Driver: "/lib/psqlodbcw.so"})
		c.SetODBCDriver(config.ODBCDriver{Name: "Etc", Driver: "/etc/psqlodbcw.so"})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}
	if dirs := ODBCDriverDirs(); len(dirs) != 0 {
		t.Errorf("ODBCDriverDirs() = %v, want none of the container's own runtime directories", dirs)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	for _, bad := range []string{"Volume=/usr/lib:", "Volume=/lib:", "Volume=/etc:"} {
		if strings.Contains(content, bad) {
			t.Errorf("FPM quadlet mounts over the container's runtime (%s):\n%s", bad, content)
		}
	}

	franken, err := GenerateFrankenPHPQuadlet("myapp", filepath.Join(tmp, "myapp"), "8.4", nil, nil)
	if err != nil {
		t.Fatalf("GenerateFrankenPHPQuadlet: %v", err)
	}
	if strings.Contains(franken, "Volume=/usr/lib:") {
		t.Errorf("FrankenPHP quadlet mounts over the container's runtime:\n%s", franken)
	}
}

// The guard has to stay narrow. /usr/lib64 is where a Fedora driver lives and
// the Alpine image has no such directory, and /opt is where a vendor client is
// normally unpacked, so both still mount.
func TestODBCDirShadowsRuntimeLeavesUsableDirsAlone(t *testing.T) {
	for _, dir := range []string{"/usr/lib64", "/opt", "/opt/hana/hdbclient", "/srv/drivers", "/home/me/drv"} {
		if ODBCDirShadowsRuntime(dir) {
			t.Errorf("ODBCDirShadowsRuntime(%q) = true, want the directory to stay mountable", dir)
		}
	}
	for _, dir := range []string{"/usr/lib", "/lib", "/usr/local/lib", "/etc", "/var", "/", "/usr/lib/"} {
		if !ODBCDirShadowsRuntime(dir) {
			t.Errorf("ODBCDirShadowsRuntime(%q) = false, want it refused", dir)
		}
	}
}

// The driver manager opens exactly the path odbcinst.ini names, so the mount has
// to be written at that spelling and not at what it resolves to. macOS reaches
// its temp directories and /var through a symlink, which made the FPM quadlet
// mount a resolved path the registry never points at while the FrankenPHP
// quadlet mounted the stored one, so the two disagreed about the same driver.
func TestQuadletsMountADriverDirAtTheSpellingTheRegistryUses(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	real := filepath.Join(root, "real", "drv")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	stored := filepath.Join(root, "link", "drv", "libodbcHDB.so")
	storedDir := filepath.Dir(stored)
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: stored})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	want := "Volume=" + storedDir + ":" + storedDir + ":ro"
	fpm, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if !strings.Contains(fpm, want) {
		t.Errorf("FPM quadlet does not mount the driver dir as the registry names it (%s):\n%s", want, fpm)
	}
	if resolved := "Volume=" + real + ":" + real; strings.Contains(fpm, resolved) {
		t.Errorf("FPM quadlet mounted the resolved path, which the registry never names:\n%s", fpm)
	}

	franken, err := GenerateFrankenPHPQuadlet("myapp", filepath.Join(root, "myapp"), "8.4", nil, nil)
	if err != nil {
		t.Fatalf("GenerateFrankenPHPQuadlet: %v", err)
	}
	if !strings.Contains(franken, want) {
		t.Errorf("FrankenPHP quadlet disagrees with the FPM one about the same driver (%s):\n%s", want, franken)
	}
}

// The probe mounts the generated registry and the driver, both out of the home
// directory, so on an SELinux distribution it has to opt out of labelling the
// way every other lerd container does. Without it the registry reads as empty
// and a driver that loads perfectly is reported as one the image cannot see.
func TestODBCProbeOptsOutOfSELinuxLabelling(t *testing.T) {
	var got []string
	restore := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		got = args
		return exec.Command("true")
	}
	defer func() { execCommand = restore }()

	_, _ = InspectODBCDriver("8.4", config.ODBCDriver{Name: "HDBODBC", Driver: "/opt/hana/libodbcHDB.so"})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "label=disable") {
		t.Errorf("probe run = %q, want the SELinux opt-out the quadlets use", joined)
	}
	if !strings.Contains(joined, "/etc/odbcinst.ini:ro") {
		t.Errorf("probe run = %q, want the registry still mounted", joined)
	}
}
