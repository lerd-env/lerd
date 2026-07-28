package cli

import (
	"runtime"
	"strings"
	"testing"
)

// --unattended pairs with `lerd bootstrap`, which only exists on Linux: it is
// what applies the root-level steps and the CA trust that the unattended run
// deliberately skips. Off Linux it would quietly produce an install with no
// resolver grant and an untrusted CA, so it is refused instead.
func TestUnattendedRefusedOffLinux(t *testing.T) {
	err := checkUnattendedSupported(true)
	if runtime.GOOS == "linux" {
		if err != nil {
			t.Fatalf("checkUnattendedSupported() = %v; want it allowed on Linux", err)
		}
		return
	}
	if err == nil {
		t.Fatal("checkUnattendedSupported() = nil; want a refusal off Linux")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error %q does not mention what the flag pairs with", err)
	}
}

func TestUnattendedNotRequestedIsAlwaysFine(t *testing.T) {
	if err := checkUnattendedSupported(false); err != nil {
		t.Fatalf("checkUnattendedSupported(false) = %v; want nil", err)
	}
}
