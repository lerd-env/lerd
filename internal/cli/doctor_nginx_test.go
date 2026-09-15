package cli

import "testing"

// The bug this check exists for: nginx stopped, no container, every site
// answering nothing, and the doctor reporting "All critical checks passed"
// because it only ever asked whether the unit was installed.
func TestNginxServiceFinding_installedButStoppedIsAFailure(t *testing.T) {
	detail, hint, healthy := nginxServiceFinding(true, false)
	if healthy {
		t.Fatal("a stopped nginx reported healthy")
	}
	if detail == "" || hint != "run: lerd start" {
		t.Errorf("detail = %q, hint = %q, want the stopped case to point at lerd start", detail, hint)
	}
}

func TestNginxServiceFinding_notInstalledPointsAtInstall(t *testing.T) {
	_, hint, healthy := nginxServiceFinding(false, false)
	if healthy {
		t.Fatal("a missing nginx unit reported healthy")
	}
	if hint != "run: lerd install" {
		t.Errorf("hint = %q, want it to point at lerd install", hint)
	}
}

func TestNginxServiceFinding_runningIsHealthy(t *testing.T) {
	if _, _, healthy := nginxServiceFinding(true, true); !healthy {
		t.Error("a running nginx reported unhealthy")
	}
}
