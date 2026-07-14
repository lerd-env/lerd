package podman

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// resetPathMountAttempts clears the debounce cache so tests can drive the
// guards in isolation.
func resetPathMountAttempts() {
	pathMountAttemptsMu.Lock()
	pathMountAttempts = map[string]time.Time{}
	pathMountAttemptsMu.Unlock()
}

func TestEphemeralPathsAreSkipped(t *testing.T) {
	cases := []string{
		"/tmp/ide-phpinfo.php",
		"/var/tmp/foo",
		"/run/whatever",
		"/proc/self",
		"/sys/something",
		"/dev/null",
	}
	for _, p := range cases {
		matched := false
		for _, prefix := range ephemeralPathPrefixes {
			if len(p) >= len(prefix) && p[:len(prefix)] == prefix {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("%s should be classified as ephemeral", p)
		}
	}
}

func TestPathAutoMountable(t *testing.T) {
	cases := map[string]bool{
		"/srv/projects":    true,
		"/mnt/data/apps":   true,
		"/":                false,
		"":                 false,
		"relative/path":    false,
		"/var/tmp/lerd":    false,
		"/tmp/lerd":        false,
		"/run/user/1000/x": false,
	}
	for path, want := range cases {
		if got := PathAutoMountable(path); got != want {
			t.Errorf("PathAutoMountable(%q) = %v, want %v", path, got, want)
		}
	}
}

// A path lerd refuses to auto-mount can still be reachable inside the container
// because it was parked: the Volume line is already in the FPM quadlet.
func TestPathVisible(t *testing.T) {
	home := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	quadlets := filepath.Join(cfgHome, "containers", "systemd")
	if err := os.MkdirAll(quadlets, 0755); err != nil {
		t.Fatal(err)
	}
	content := "[Container]\nVolume=/var/tmp/parked:/var/tmp/parked:rw\nVolume=/srv/apps:/srv/apps:rw\n"
	if err := os.WriteFile(filepath.Join(quadlets, "lerd-php84-fpm.container"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]bool{
		filepath.Join(home, "Code", "app"): true,  // under $HOME
		"/var/tmp/parked":                  true,  // parked, mounted verbatim
		"/var/tmp/parked/nested":           true,  // covered by the parked ancestor
		"/var/tmp/other":                   false, // never mounted
		"/srv/apps/shop":                   true,
		"/srv/appsuite":                    false, // ancestor match must respect the separator
		"/opt/elsewhere":                   false,
	}
	for path, want := range cases {
		if got := PathVisible(path, "8.4"); got != want {
			t.Errorf("PathVisible(%q) = %v, want %v", path, got, want)
		}
	}

	if PathVisible("/srv/apps/shop", "8.1") {
		t.Error("PathVisible should report false when the version's quadlet does not exist")
	}
}

func TestPathMountDebounce_BlocksRecentRetries(t *testing.T) {
	resetPathMountAttempts()
	t.Cleanup(resetPathMountAttempts)

	const path = "/srv/myapp"
	// First record: simulate an attempt happening now.
	pathMountAttemptsMu.Lock()
	pathMountAttempts[path] = time.Now()
	pathMountAttemptsMu.Unlock()

	pathMountAttemptsMu.Lock()
	last, ok := pathMountAttempts[path]
	pathMountAttemptsMu.Unlock()
	if !ok || time.Since(last) >= pathMountDebounce {
		t.Errorf("expected fresh entry to be within debounce window")
	}
}

func TestPathMountDebounce_ExpiresAfterWindow(t *testing.T) {
	resetPathMountAttempts()
	t.Cleanup(resetPathMountAttempts)

	const path = "/srv/myapp"
	pathMountAttemptsMu.Lock()
	pathMountAttempts[path] = time.Now().Add(-2 * pathMountDebounce)
	pathMountAttemptsMu.Unlock()

	pathMountAttemptsMu.Lock()
	last, ok := pathMountAttempts[path]
	pathMountAttemptsMu.Unlock()
	if !ok {
		t.Fatal("entry should still be present in the map until next access")
	}
	if time.Since(last) < pathMountDebounce {
		t.Errorf("entry should be older than the debounce window; got age=%v", time.Since(last))
	}
}
