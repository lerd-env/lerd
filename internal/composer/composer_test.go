package composer

import (
	"os"
	"testing"
)

func TestProcessTimeoutEnv_DefaultWhenUnset(t *testing.T) {
	if err := os.Unsetenv("COMPOSER_PROCESS_TIMEOUT"); err != nil {
		t.Fatal(err)
	}
	got := ProcessTimeoutEnv()
	want := "COMPOSER_PROCESS_TIMEOUT=" + DefaultProcessTimeout
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProcessTimeoutEnv_HostOverrideWins(t *testing.T) {
	t.Setenv("COMPOSER_PROCESS_TIMEOUT", "0")
	if got := ProcessTimeoutEnv(); got != "COMPOSER_PROCESS_TIMEOUT=0" {
		t.Errorf("host override not honoured: got %q", got)
	}
}

func TestProcessTimeoutEnv_EmptyHostValueFallsBack(t *testing.T) {
	t.Setenv("COMPOSER_PROCESS_TIMEOUT", "")
	got := ProcessTimeoutEnv()
	want := "COMPOSER_PROCESS_TIMEOUT=" + DefaultProcessTimeout
	if got != want {
		t.Errorf("empty host value should fall back to default: got %q", got)
	}
}
