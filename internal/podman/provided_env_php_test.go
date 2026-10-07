package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDumpBridge_LoadsProvidedEnv runs the real bridge as a prepend and checks
// it loads the site's provided-env file, unquotes values (multi-line quoted
// ones too), drops inline comments, keeps variables the
// process already has, and loads nothing for a LERD_SITE that walks out of the
// dir or a script outside the roots the file names.
func TestDumpBridge_LoadsProvidedEnv(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	bridge, err := DumpBridgePHP()
	if err != nil {
		t.Fatalf("DumpBridgePHP: %v", err)
	}
	dir := t.TempDir()
	bridgePath := filepath.Join(dir, "dump-bridge.php")
	envDir := filepath.Join(dir, "env")
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bridgePath, []byte(bridge), 0o644); err != nil {
		t.Fatal(err)
	}
	siteDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	otherDir := t.TempDir()
	env := "#lerd-root=" + siteDir + "\n# comment\nPLAIN=one\nSINGLE='two words'\nexport DQ=\"a\\nb\"\nKEPT=from-file\n" +
		"PEM=\"-----BEGIN KEY-----\nabc\n-----END KEY-----\"\nNOTE=bar # note\nHASH=a#b\n#lerd-root=/\n"
	if err := os.WriteFile(filepath.Join(envDir, "app.env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.env"), []byte("PLAIN=escaped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `<?php echo json_encode(array(getenv("PLAIN"), $_ENV["SINGLE"] ?? null, $_SERVER["DQ"] ?? null, getenv("KEPT"), getenv("PEM"), getenv("NOTE"), getenv("HASH")));`
	for _, d := range []string{siteDir, otherDir} {
		if err := os.WriteFile(filepath.Join(d, "probe.php"), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	preflight := filepath.Join(dir, "preflight.php")
	if err := os.WriteFile(preflight, []byte("<?php echo file_exists("+phpQuote(bridgePath)+") ? 'Y' : 'N';"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command(php, "-n", preflight).CombinedOutput(); !strings.Contains(string(out), "Y") {
		t.Skip("php cannot read host files (containerised/sandboxed wrapper); native php needed")
	}

	run := func(site, probeDir string) string {
		probe := filepath.Join(probeDir, "probe.php")
		cmd := exec.Command(php, "-n", "-d", "auto_prepend_file="+bridgePath, "-d", "lerd.provided_env_dir="+envDir, probe)
		cmd.Env = append(os.Environ(), "LERD_SITE="+site, "KEPT=from-process")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("php: %v\n%s", err, out)
		}
		return strings.TrimSpace(string(out))
	}

	nothing := `[false,null,null,"from-process",false,false,false]`
	if out, want := run("app", siteDir), `["one","two words","a\nb","from-process","-----BEGIN KEY-----\nabc\n-----END KEY-----","bar","a#b"]`; out != want {
		t.Errorf("loaded env = %s, want %s", out, want)
	}
	if out := run("../secret", siteDir); out != nothing {
		t.Errorf("a traversing LERD_SITE must load nothing, got %s", out)
	}
	if out := run("app", otherDir); out != nothing {
		t.Errorf("a script outside the file's roots must load nothing, got %s", out)
	}
}

// TestFPMQuadlet_ProvidedEnvLinesPerPlatform checks the FPM unit mounts the
// provided-env dir read-only where it exists, and nothing where it does not.
func TestFPMQuadlet_ProvidedEnvLinesPerPlatform(t *testing.T) {
	cases := []struct {
		name          string
		goos          string
		native        bool
		xdg           string
		mount, prestr string
	}{
		{"linux", "linux", false, "/run/user/1000", "Volume=%t/lerd/env:/run/lerd/env:ro", "ExecStartPre=/bin/mkdir -p -m 0700 %t/lerd/env"},
		{"linux without runtime dir", "linux", false, "", "", ""},
		{"macOS container runtime", "darwin", false, "", "Volume=/run/lerd/env:/run/lerd/env:ro", ""},
		{"macOS native runtime", "darwin", true, "", "", ""},
		{"other", "windows", false, "", "", ""},
	}
	for _, c := range cases {
		mount, pre := providedEnvLinesFor(c.goos, c.native, c.xdg)
		if mount != c.mount || pre != c.prestr {
			t.Errorf("%s: got %q %q, want %q %q", c.name, mount, pre, c.mount, c.prestr)
		}
	}
}
