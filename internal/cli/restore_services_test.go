package cli

import (
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A site's .lerd.yaml still listing a service the user removed must not bring
// it back on the next start; each name is also restored only once across sites.
func TestServicesToRestore_skipsRemovedAndSeen(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	if err := config.SetServiceRemoved("mailpit", true); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{"mysql": true}
	got := servicesToRestore([]config.ProjectService{{Name: "mysql"}, {Name: "mailpit"}, {Name: "redis"}}, seen)
	if len(got) != 1 || got[0].Name != "redis" {
		t.Errorf("restored %v, want only redis", got)
	}
	if !seen["redis"] {
		t.Error("redis not marked seen, a second site would restore it again")
	}
}
