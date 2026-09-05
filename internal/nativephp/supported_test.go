package nativephp

import (
	"strings"
	"testing"
)

// 7.4 and 8.0 have no native runtime and never will: static PHP cannot carry
// OPcache below 8.0, and both fail to compile against the modern libxml2 and
// ICU the toolchain builds with. Saying "not installed" would send someone
// looking for a download that does not exist.
func TestSupportedRejectsTheLegacyTier(t *testing.T) {
	for _, v := range []string{"7.4", "8.0"} {
		if Supported(v) {
			t.Errorf("php %s must not be reported as supported natively", v)
		}
	}
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4", "8.5"} {
		if !Supported(v) {
			t.Errorf("php %s builds natively and must be supported", v)
		}
	}
}

// The refusal has to say the version cannot run natively, not that a file is
// missing, or the reader waits for a build that is never coming.
func TestEnsureInstalledExplainsAnUnsupportedVersion(t *testing.T) {
	err := EnsureInstalled("7.4", "/nonexistent/php-native-fpm-7.4")
	if err == nil {
		t.Fatal("expected an error for an unsupported version")
	}
	msg := err.Error()
	if !strings.Contains(msg, "7.4") || !strings.Contains(msg, MinVersion) {
		t.Errorf("error should name the version and the minimum, got: %v", err)
	}
	if strings.Contains(msg, "not installed") {
		t.Errorf("an unsupported version is not a missing download, got: %v", err)
	}
}
