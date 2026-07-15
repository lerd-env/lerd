package config

import "testing"

// IsBuiltinWorker must recognise the lerd-managed workers that live outside any
// framework's worker definitions, so a validator does not flag them as undefined.
func TestIsBuiltinWorker(t *testing.T) {
	for _, name := range []string{StripeWorkerName, HostProxyWorkerName} {
		if !IsBuiltinWorker(name) {
			t.Errorf("IsBuiltinWorker(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"queue", "horizon", "schedule", "vite", "reverb", ""} {
		if IsBuiltinWorker(name) {
			t.Errorf("IsBuiltinWorker(%q) = true, want false", name)
		}
	}
}
