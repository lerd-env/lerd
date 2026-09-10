package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// UIClient{Network,Addr} must give the CLI a transport that actually exists on
// the host: macOS has no lerd-ui unix socket (server binds it Linux-only), so
// the client dials the TCP loopback; Linux stays on the unix socket.
func TestUIClientTransport_matchesOS(t *testing.T) {
	net, addr := UIClientNetwork(), UIClientAddr()
	if runtime.GOOS == "darwin" {
		if net != "tcp" {
			t.Errorf("UIClientNetwork() = %q on darwin, want tcp", net)
		}
		if addr != "127.0.0.1:7073" {
			t.Errorf("UIClientAddr() = %q on darwin, want 127.0.0.1:7073", addr)
		}
		return
	}
	if net != "unix" {
		t.Errorf("UIClientNetwork() = %q on %s, want unix", net, runtime.GOOS)
	}
	if addr != UISocketPath() {
		t.Errorf("UIClientAddr() = %q, want UISocketPath() %q", addr, UISocketPath())
	}
}

// AccessLogTarget must give nginx a syslog server it can reach: macOS ships the
// feed to host.containers.internal over gvproxy UDP (nginx is in the VM), Linux
// stays on the bind-mounted unix socket. The watcher's listen addr must pair.
func TestAccessLogTarget_matchesOS(t *testing.T) {
	target := AccessLogTarget()
	if runtime.GOOS == "darwin" {
		want := "host.containers.internal:" + AccessFeedUDPPort
		if target != want {
			t.Errorf("AccessLogTarget() = %q on darwin, want %q", target, want)
		}
		if addr := AccessFeedListenAddr(); addr != "127.0.0.1:"+AccessFeedUDPPort {
			t.Errorf("AccessFeedListenAddr() = %q on darwin, want 127.0.0.1:%s", addr, AccessFeedUDPPort)
		}
		return
	}
	if want := "unix:" + AccessSocketPath(); target != want {
		t.Errorf("AccessLogTarget() = %q on %s, want %q", target, runtime.GOOS, want)
	}
}

// ── XDG overrides ─────────────────────────────────────────────────────────────

func TestConfigDir_UsesXDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got := ConfigDir()
	want := filepath.Join(tmp, "lerd")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestDataDir_UsesXDGDataHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got := DataDir()
	want := filepath.Join(tmp, "lerd")
	if got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

// ── Path suffix correctness ───────────────────────────────────────────────────

func TestPathFunctions_ContainExpectedSuffixes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cases := []struct {
		name   string
		got    string
		suffix string
	}{
		{"BinDir", BinDir(), "lerd/bin"},
		{"NginxDir", NginxDir(), "lerd/nginx"},
		{"NginxConfD", NginxConfD(), filepath.Join("nginx", "conf.d")},
		{"CertsDir", CertsDir(), "lerd/certs"},
		{"DnsmasqDir", DnsmasqDir(), "lerd/dnsmasq"},
		{"SitesFile", SitesFile(), "sites.yaml"},
		{"GlobalConfigFile", GlobalConfigFile(), "config.yaml"},
		{"QuadletDir", QuadletDir(), filepath.Join("containers", "systemd")},
		{"SystemdUserDir", SystemdUserDir(), filepath.Join("systemd", "user")},
		{"CustomServicesDir", CustomServicesDir(), filepath.Join("lerd", "services")},
		{"FrameworksDir", FrameworksDir(), filepath.Join("lerd", "frameworks")},
		{"UpdateCheckFile", UpdateCheckFile(), "update-check.json"},
		{"PausedDir", PausedDir(), "lerd/paused"},
	}

	for _, c := range cases {
		if !strings.HasSuffix(c.got, c.suffix) {
			t.Errorf("%s() = %q, expected suffix %q", c.name, c.got, c.suffix)
		}
	}
}

func TestPHPConfFile_ContainsVersionAndXdebug(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got := PHPConfFile("8.3")
	if !strings.Contains(got, "8.3") {
		t.Errorf("PHPConfFile(8.3) = %q, expected to contain version", got)
	}
	if !strings.HasSuffix(got, "99-xdebug.ini") {
		t.Errorf("PHPConfFile(8.3) = %q, expected suffix 99-xdebug.ini", got)
	}
}

func TestPHPUserIniFile_ContainsVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got := PHPUserIniFile("8.4")
	if !strings.Contains(got, "8.4") {
		t.Errorf("PHPUserIniFile(8.4) = %q, expected to contain version", got)
	}
	if !strings.HasSuffix(got, "98-user.ini") {
		t.Errorf("PHPUserIniFile(8.4) = %q, expected suffix 98-user.ini", got)
	}
}

func TestSharedIniFile_IsVersionAgnosticAndSortsBelowUserIni(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got := SharedIniFile()
	if !strings.HasSuffix(got, filepath.Join("php", "shared", "95-shared.ini")) {
		t.Errorf("SharedIniFile() = %q, expected suffix php/shared/95-shared.ini", got)
	}
	// The shared file must load before the per-version 98-user.ini so the
	// per-version value wins on a conflicting key. conf.d loads alphabetically,
	// so the basename must sort earlier.
	shared := filepath.Base(SharedIniFile())
	perVersion := filepath.Base(PHPUserIniFile("8.4"))
	if !(shared < perVersion) {
		t.Errorf("shared ini basename %q must sort before per-version %q", shared, perVersion)
	}
}

func TestDataSubDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got := DataSubDir("mysql")
	if !strings.Contains(got, "mysql") {
		t.Errorf("DataSubDir(mysql) = %q, expected to contain mysql", got)
	}
}

// A command string from a framework definition starts with `php` (php spark,
// php please, php bin/magento, php vendor/drush/drush/drush.php). Those resolve
// through lerd's shim dir, which `lerd path:disable` deliberately keeps off the
// user's shell PATH, so every caller that hands such a string to a shell has to
// put the dir back for the child process.
func TestPathWithBinDir_PrependsShimDir(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	got := PathWithBinDir()
	sep := string(os.PathListSeparator)
	if !strings.HasPrefix(got, BinDir()+sep) {
		t.Errorf("PathWithBinDir() = %q, want it to start with %q", got, BinDir())
	}
	if !strings.HasSuffix(got, sep+"/usr/bin:/bin") {
		t.Errorf("PathWithBinDir() = %q, want it to end with the inherited PATH", got)
	}
}

// A bare "PATH=<bin>:" searches the working directory on POSIX, so an empty
// inherited PATH must not gain a trailing separator.
func TestPathWithBinDir_EmptyPathHasNoTrailingSeparator(t *testing.T) {
	t.Setenv("PATH", "")
	got := PathWithBinDir()
	if strings.HasSuffix(got, string(os.PathListSeparator)) {
		t.Errorf("PathWithBinDir() = %q, want no trailing separator", got)
	}
	if !strings.HasPrefix(got, BinDir()) {
		t.Errorf("PathWithBinDir() = %q, want it to start with %q", got, BinDir())
	}
}

// A doctor fix, a worker or a custom command may call `lerd` itself. The
// daemons that run those are started by launchd, whose PATH is the system
// default and never carries ~/.local/bin, so the binary has to put its own
// directory on the PATH it hands to a child.
func TestPathWithBinDir_IncludesExecutableDir(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path on this platform")
	}
	dir := filepath.Dir(exe)
	found := false
	for _, p := range strings.Split(PathWithBinDir(), string(os.PathListSeparator)) {
		if p == dir {
			found = true
		}
	}
	if !found {
		t.Errorf("PathWithBinDir() = %q, want it to contain %q", PathWithBinDir(), dir)
	}
}
