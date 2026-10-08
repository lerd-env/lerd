package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The profiler names the bridge as its script while the bridge is also the
// prepend, so the bridge has to survive being loaded twice in one request.
func TestDumpBridge_LoadsTwiceWithoutFataling(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	bridge, err := DumpBridgePHP()
	if err != nil {
		t.Fatalf("DumpBridgePHP: %v", err)
	}
	collector, err := DevtoolsCollectorPHP()
	if err != nil {
		t.Fatalf("DevtoolsCollectorPHP: %v", err)
	}
	dir := t.TempDir()
	bridgePath := filepath.Join(dir, "dump-bridge.php")
	for name, body := range map[string]string{"dump-bridge.php": bridge, "devtools-collector.php": collector, "enabled.flag": "1\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
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

	out, err := exec.Command(php, "-n", "-d", "auto_prepend_file="+bridgePath, "-d", "lerd.assets_dir="+dir, bridgePath).CombinedOutput()
	if err != nil {
		t.Fatalf("second load failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "Fatal") {
		t.Errorf("second load fataled:\n%s", out)
	}
}
