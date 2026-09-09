package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const nativePin = `tools:
  php-native-8.4:
    version: "8.4.24"
    url: https://github.com/lerd-env/php/releases/download/php-8.4.24/{asset}
    assets:
      darwin/arm64: lerd-php-8.4.24-darwin-arm64.tar.gz
    digests:
      darwin/arm64: d6fc7f0e595cf3e84479f524c5b7ee95eb31b5acf359b984fc302c01c6e0da66
    sizes:
      darwin/arm64: 67312315
`

// The native builds are published by a different repository on its own
// schedule, so their pins arrive from a second manifest and have to merge with
// the first rather than replace it.
func TestLoadMergesTheNativePHPManifest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	onDarwin(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(nativePin)) //nolint:errcheck
	}))
	defer srv.Close()
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml") // unreachable: use embedded
	t.Setenv("LERD_NATIVE_PHP_URL", srv.URL)

	m := Load(context.Background())
	got, ok := m.Tools["php-native-8.4"]
	if !ok {
		t.Fatal("the native pin did not reach the manifest")
	}
	if got.Version != "8.4.24" {
		t.Errorf("version = %q, want 8.4.24", got.Version)
	}
	if d := m.Digest("php-native-8.4", "darwin", "arm64"); len(d) != 64 {
		t.Errorf("digest = %q, want a sha256", d)
	}
	// The embedded pins must survive: this is a merge, not a replacement.
	if _, ok := m.Tools["composer"]; !ok {
		t.Error("the native manifest displaced the embedded tools")
	}
}

// A native manifest that cannot be reached must leave the rest of the pins
// working, the same way an unreachable tools manifest does.
func TestLoadSurvivesAnAbsentNativeManifest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	onDarwin(t)
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	t.Setenv("LERD_NATIVE_PHP_URL", "http://127.0.0.1:1/native-php.yaml")
	if _, ok := Load(context.Background()).Tools["composer"]; !ok {
		t.Error("an unreachable native manifest broke the embedded pins")
	}
}

// onDarwin pretends this is a Mac, which is where the native pins are fetched.
func onDarwin(t *testing.T) {
	t.Helper()
	prev := nativePinsGOOS
	nativePinsGOOS = "darwin"
	t.Cleanup(func() { nativePinsGOOS = prev })
}

// The builds exist for macOS only, so every other platform would be fetching a
// manifest it can never install anything from, once a day, forever.
func TestNativePinsAreNotFetchedOffDarwin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Write([]byte(nativePin)) //nolint:errcheck
	}))
	defer srv.Close()
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	t.Setenv("LERD_NATIVE_PHP_URL", srv.URL)

	prev := nativePinsGOOS
	nativePinsGOOS = "linux"
	defer func() { nativePinsGOOS = prev }()

	if _, ok := Load(context.Background()).Tools["php-native-8.4"]; ok {
		t.Error("the native pins reached a platform with no builds")
	}
	if hits != 0 {
		t.Errorf("the native manifest was fetched %d times off darwin", hits)
	}
}
