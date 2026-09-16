package phpini

import "testing"

// Applying a shared ini walks every registered PHP version. Under the native
// runtime a version with no host build has nothing to reload, and reaching for
// one failed the whole command: the change had already reached every version
// that serves, and the user was told it had not.
func TestVersionsToReload(t *testing.T) {
	registered := []string{"8.2", "8.3", "8.4", "8.5", "8.6"}
	built := []string{"8.3", "8.4", "8.5"}

	got := versionsToReload(registered, true, func() []string { return built })
	if len(got) != 3 || got[0] != "8.3" || got[2] != "8.5" {
		t.Errorf("native should reload only the built versions, got %v", got)
	}

	got = versionsToReload(registered, false, func() []string { return built })
	if len(got) != len(registered) {
		t.Errorf("container should reload every registered version, got %v", got)
	}
}

// With no native build at all there is nothing to reload, and that is not a
// failure either.
func TestVersionsToReloadWithNoNativeBuilds(t *testing.T) {
	if got := versionsToReload([]string{"8.4"}, true, func() []string { return nil }); len(got) != 0 {
		t.Errorf("expected nothing to reload, got %v", got)
	}
}
