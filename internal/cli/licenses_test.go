package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLicensesCmdPrintsTheEmbeddedNotices(t *testing.T) {
	cmd := NewLicensesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "# Third-party licenses") {
		t.Fatalf("output does not start with the notices header: %.80q", got)
	}
	// The point of the command is that the license bodies travel with the
	// binary, so a summary-sized file means the generator did not run.
	if len(got) < 50_000 {
		t.Errorf("embedded notices are only %d bytes, run `make licenses`", len(got))
	}
	for _, want := range []string{"Go modules linked into the lerd binaries", "npm packages used to build the embedded web UI"} {
		if !strings.Contains(got, want) {
			t.Errorf("notices are missing the %q section", want)
		}
	}
}
