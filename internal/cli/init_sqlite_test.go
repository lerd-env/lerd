package cli

import "testing"

// The wizard's database pick becomes a .lerd.yaml services entry. SQLite must
// not: it has no preset, no container and nothing to install, and the entry is
// what made a site page offer to install it.
func TestPersistedServices_leavesSQLiteOut(t *testing.T) {
	got := persistedServices("sqlite", []string{"mailpit", "redis"})
	for _, s := range got {
		if s == "sqlite" {
			t.Errorf("services = %v, want sqlite left out", got)
		}
	}
	if len(got) != 2 {
		t.Errorf("services = %v, want the other picks kept", got)
	}
}

func TestPersistedServices_keepsARealDatabase(t *testing.T) {
	got := persistedServices("postgres", []string{"mailpit"})
	if len(got) != 2 || got[0] != "postgres" {
		t.Errorf("services = %v, want the database first, then the rest", got)
	}
}

func TestPersistedServices_handlesNoPickAtAll(t *testing.T) {
	if got := persistedServices("", nil); len(got) != 0 {
		t.Errorf("services = %v, want none", got)
	}
}
