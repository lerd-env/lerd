package cli

import (
	"strings"
	"testing"
)

func TestBrowserTrustGuidance_atomic(t *testing.T) {
	msg := browserTrustGuidance(true)
	for _, want := range []string{"certutil", "rpm-ostree install nss-tools", "reboot", "lerd dns:repair", "lerd dns:disable"} {
		if !strings.Contains(msg, want) {
			t.Errorf("atomic guidance missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "apt install") || strings.Contains(msg, "pacman") {
		t.Errorf("atomic guidance should not offer non-atomic package managers: %s", msg)
	}
}

func TestBrowserTrustGuidance_ordinary(t *testing.T) {
	msg := browserTrustGuidance(false)
	for _, want := range []string{"certutil", "nss-tools", "lerd dns:repair", "lerd dns:disable"} {
		if !strings.Contains(msg, want) {
			t.Errorf("guidance missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "rpm-ostree") {
		t.Errorf("ordinary guidance should not tell the user to layer with rpm-ostree: %s", msg)
	}
}
