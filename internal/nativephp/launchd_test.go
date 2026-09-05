package nativephp

import (
	"strings"
	"testing"
)

func TestPlistCarriesEnvAndArgs(t *testing.T) {
	got := buildFPMPlist("lerd-native-php84",
		[]string{"/bin/php-fpm", "-y", "/conf/php-fpm.conf"},
		map[string]string{"PHP_INI_SCAN_DIR": "/a:/b"},
		"/logs/native84.log")

	for _, want := range []string{
		"<string>lerd-native-php84</string>",
		"<string>/bin/php-fpm</string>",
		"<string>-y</string>",
		"<key>PHP_INI_SCAN_DIR</key>",
		"<string>/a:/b</string>",
		"<key>KeepAlive</key>",
		"<key>RunAtLoad</key>",
		"/logs/native84.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q:\n%s", want, got)
		}
	}
}

// A path with an ampersand must not produce invalid XML that launchd rejects.
func TestPlistEscapesXML(t *testing.T) {
	got := buildFPMPlist("l", []string{"/bin/a&b"}, map[string]string{"K": "<v>"}, "/log")
	if strings.Contains(got, "/bin/a&b") || strings.Contains(got, "<v>") {
		t.Errorf("plist must escape XML metacharacters:\n%s", got)
	}
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "&lt;v&gt;") {
		t.Errorf("expected escaped forms:\n%s", got)
	}
}

// Env order must be stable or every regeneration rewrites the plist and
// needlessly restarts FPM.
func TestPlistIsDeterministic(t *testing.T) {
	env := map[string]string{"B": "2", "A": "1", "C": "3"}
	a := buildFPMPlist("l", []string{"/bin/x"}, env, "/log")
	b := buildFPMPlist("l", []string{"/bin/x"}, env, "/log")
	if a != b {
		t.Error("plist generation must be deterministic")
	}
}

func TestUnitLabel(t *testing.T) {
	if got := UnitLabel("8.4"); got != "lerd-native-php84" {
		t.Errorf("UnitLabel(8.4) = %q, want lerd-native-php84", got)
	}
	if UnitLabel("8.4") == UnitLabel("8.5") {
		t.Error("each version needs its own launchd label")
	}
	// Must not collide with the containerised FPM unit names.
	if strings.Contains(UnitLabel("8.4"), "-fpm") {
		t.Errorf("label %q is too close to the container unit name", UnitLabel("8.4"))
	}
}

// Ensure is called on every start, not just on a switch, so it has to be a
// no-op when the listener is already up. Bootstrapping an already-loaded job
// fails with "Bootstrap failed: 5", which surfaced as a warning per version on
// a perfectly healthy install.
func TestBootstrapIsSkippedWhenAlreadyLoaded(t *testing.T) {
	var booted []string
	loaded := true
	err := bootLaunchdUnitWith("lerd-native-php84", "/tmp/x.plist",
		func(string) bool { return loaded },
		func(label, path string) error { booted = append(booted, label); return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(booted) != 0 {
		t.Errorf("an already-loaded job must not be bootstrapped again, got %v", booted)
	}

	loaded = false
	if err := bootLaunchdUnitWith("lerd-native-php84", "/tmp/x.plist",
		func(string) bool { return loaded },
		func(label, path string) error { booted = append(booted, label); return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(booted) != 1 {
		t.Errorf("an absent job must be bootstrapped, got %v", booted)
	}
}

// An ini change only reaches requests once FPM reloads, and Ensure is
// deliberately a no-op for an already-running listener, so the ini path needs
// a restart rather than an ensure.
func TestReloadRestartsARunningListener(t *testing.T) {
	var kicked []string
	err := reloadWith("lerd-native-php84",
		func(string) bool { return true },
		func(label string) error { kicked = append(kicked, label); return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kicked) != 1 {
		t.Errorf("a running listener must be restarted, got %v", kicked)
	}
}
