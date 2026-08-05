package podman

import "testing"

func TestFPMUnitName(t *testing.T) {
	for version, want := range map[string]string{
		"8.5": "lerd-php85-fpm",
		"8.4": "lerd-php84-fpm",
		"7.4": "lerd-php74-fpm",
	} {
		if got := FPMUnitName(version); got != want {
			t.Errorf("FPMUnitName(%q) = %q, want %q", version, got, want)
		}
	}
}

// The probe has to run inside the container and must not depend on tooling an
// FPM image may not carry, so it goes through PHP itself.
func TestFPMReadyProbeUsesPHP(t *testing.T) {
	if len(fpmReadyProbe) == 0 || fpmReadyProbe[0] != "php" {
		t.Fatalf("probe should run through php, got %v", fpmReadyProbe)
	}
	joined := ""
	for _, a := range fpmReadyProbe {
		joined += a + " "
	}
	if !contains(joined, "9000") {
		t.Errorf("probe does not test the fastcgi port: %v", fpmReadyProbe)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
