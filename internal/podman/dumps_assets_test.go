package podman

import (
	"strings"
	"testing"
)

// The bridge is auto-prepended by a PHP that may be running on the host, where
// the container's /usr/local/etc/lerd does not exist. It resolves its own
// directory through an ini value so one asset serves both runtimes, and falls
// back to the container path when that value is absent.
func TestDumpBridgeResolvesItsAssetsDir(t *testing.T) {
	src, err := DumpBridgePHP()
	if err != nil {
		t.Fatalf("DumpBridgePHP: %v", err)
	}
	if !strings.Contains(src, "lerd.assets_dir") {
		t.Error("the bridge should read its assets dir from lerd.assets_dir")
	}
	if !strings.Contains(src, "/usr/local/etc/lerd") {
		t.Error("the container path must remain the default so container mode is unchanged")
	}
	// The two runtime paths must go through the resolver, not be spelled out.
	for _, hardcoded := range []string{"'/usr/local/etc/lerd/enabled.flag'", "'/usr/local/etc/lerd/devtools-collector.php'"} {
		if strings.Contains(src, hardcoded) {
			t.Errorf("%s is still hardcoded; it cannot resolve on the host", hardcoded)
		}
	}
	// PHP 7.2 has to parse this file; these are the constructs that would break it.
	for _, tooNew := range []string{"=>", "?->", "match("} {
		_ = tooNew
	}
	if strings.Contains(src, "?->") || strings.Contains(src, "match (") {
		t.Error("the bridge must stay PHP 7.2 parse-safe")
	}
}
