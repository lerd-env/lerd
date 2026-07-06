package cli

import "testing"

// lerd link (secured project) and lerd setup call runSecure with a nil
// *cobra.Command, so it must not panic reading the --renew flag; it should just
// proceed to the secure toggle, which errors cleanly on an unknown site.
func TestRunSecure_NilCommandDoesNotPanic(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := runSecure(nil, []string{"nonexistent-site-xyz"}); err == nil {
		t.Fatal("expected an error for an unknown site, got nil")
	}
}
