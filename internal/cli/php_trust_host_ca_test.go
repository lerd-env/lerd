package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// on and off persist the opt-in; anything else is refused and leaves it alone.
func TestRunPhpTrustHostCA(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	for _, step := range []struct {
		mode    string
		want    bool
		wantErr bool
	}{
		{"on", true, false},
		{"yes", true, true},
		{"off", false, false},
		{"", false, true},
	} {
		err := runPhpTrustHostCA(step.mode)
		if (err != nil) != step.wantErr {
			t.Fatalf("mode %q: err = %v, wantErr %v", step.mode, err, step.wantErr)
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PHP.TrustHostCA != step.want {
			t.Errorf("after %q: TrustHostCA = %v, want %v", step.mode, cfg.PHP.TrustHostCA, step.want)
		}
	}
}
