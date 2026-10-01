package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMergeKeepsPlatformsThePublishedPinLacks(t *testing.T) {
	emb := Tool{Version: "v1", URL: "https://github.com/x/{asset}",
		Assets:  map[string]string{"linux/amd64": "a-linux", "windows/amd64": "a-win.exe"},
		Digests: map[string]string{"windows/amd64": "0000000000000000000000000000000000000000000000000000000000000001"},
		Sizes:   map[string]int64{"windows/amd64": 42}}
	pub := Tool{Version: "v1", URL: "https://github.com/x/{asset}",
		Assets: map[string]string{"linux/amd64": "a-linux-NEW"}}

	got := mergeMissingPlatforms(pub, emb)
	if got.Assets["linux/amd64"] != "a-linux-NEW" {
		t.Errorf("the published asset must win where both have one: %v", got.Assets)
	}
	if got.Assets["windows/amd64"] != "a-win.exe" || got.Sizes["windows/amd64"] != 42 || got.Digests["windows/amd64"] == "" {
		t.Errorf("the embedded windows entry was not carried over: %+v", got)
	}
	if _, ok := pub.Assets["windows/amd64"]; ok {
		t.Error("the published tool's map must not be modified in place")
	}
}

func TestMergeLeavesADifferentVersionAlone(t *testing.T) {
	emb := Tool{Version: "v1", Assets: map[string]string{"windows/amd64": "old.exe"}}
	pub := Tool{Version: "v2", Assets: map[string]string{"linux/amd64": "new"}}
	got := mergeMissingPlatforms(pub, emb)
	if _, ok := got.Assets["windows/amd64"]; ok {
		t.Errorf("an asset of another version must not leak into the pin: %v", got.Assets)
	}
}

// The path that failed in practice: the published tools.yaml has no windows
// mkcert, the embedded one does, and Load has to serve the embedded asset.
func TestLoadServesTheEmbeddedWindowsAssetWhenThePublishedPinLacksIt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	emb := embeddedManifest().Tools["mkcert"]
	old := "tools:\n  mkcert:\n    version: " + emb.Version + "\n    url: " + emb.URL + "\n    assets:\n      linux/amd64: mkcert-{version}-linux-amd64\n"
	cache := manifestCachePath()
	if err := os.MkdirAll(filepath.Dir(cache), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = os.Chtimes(cache, now, now) // fresh: served from disk, no network

	m := Load(context.Background())
	if _, err := m.URL("mkcert", "windows", "amd64"); err != nil {
		t.Fatalf("windows mkcert is not resolvable through Load: %v", err)
	}
	if _, err := m.URL("mkcert", "linux", "amd64"); err != nil {
		t.Errorf("the published linux asset was lost: %v", err)
	}
}
