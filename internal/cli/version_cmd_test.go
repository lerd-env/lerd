package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/version"
)

// `lerd version` has to answer the same thing as `lerd --version`: a second
// spelling that printed something else would be a second version to trust.
func TestVersionCmdPrintsTheVersion(t *testing.T) {
	cmd := NewVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if want := "lerd version " + version.String(); got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
	if !strings.Contains(got, version.Version) {
		t.Errorf("version output does not name the version: %q", got)
	}
}
