//go:build windows

package cli

import (
	"strings"
	"testing"
	"time"
)

// A Windows clock off the machine's breaks S3 signing and makes template
// engines think a just-edited file is older than its compiled copy.
func TestClockSkewWarning(t *testing.T) {
	vm := time.Date(2026, 10, 2, 17, 15, 0, 0, time.UTC)
	if got := clockSkewWarning(vm.Add(-2*time.Hour), vm); !strings.Contains(got, "2h0m0s behind") {
		t.Errorf("a host 2h behind: %q", got)
	}
	if got := clockSkewWarning(vm.Add(5*time.Minute), vm); !strings.Contains(got, "5m0s ahead of") {
		t.Errorf("a host 5m ahead: %q", got)
	}
	if got := clockSkewWarning(vm.Add(20*time.Second), vm); got != "" {
		t.Errorf("a few seconds is ssh latency, not skew: %q", got)
	}
}

func TestVMClockReadsTheDateHeader(t *testing.T) {
	if _, err := vmClock(""); err == nil {
		t.Error("a machine without a pipe has no clock to read")
	}
	if _, err := vmClock(`\\.\pipe\lerd-test-no-such-pipe`); err == nil {
		t.Error("a pipe nothing serves should fail")
	}
}
