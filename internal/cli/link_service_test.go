package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// SQLite is a per-project file, not a container. linkApplyServices must skip it
// rather than route it through ensureServiceRunning, which looks for a preset or
// custom service YAML and warns "custom service sqlite not found" when neither
// exists (there is no sqlite preset by design).
func TestLinkApplyServices_SkipsSQLite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	proj := &config.ProjectConfig{Services: []config.ProjectService{{Name: "sqlite"}}}
	if err := linkApplyServices(t.TempDir(), proj); err != nil {
		t.Fatalf("linkApplyServices: %v", err)
	}
	if strings.Contains(buf.String(), "sqlite") {
		t.Errorf("sqlite should be skipped silently, got output: %q", buf.String())
	}
}
