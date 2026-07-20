package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/cleanup"
)

func TestCleanupScopeFollowsSafeFlag(t *testing.T) {
	if got := cleanupScope(false); got != cleanup.ScopeDeep {
		t.Fatalf("default scope = %v, want ScopeDeep", got)
	}
	if got := cleanupScope(true); got != cleanup.ScopeSafe {
		t.Fatalf("--safe scope = %v, want ScopeSafe", got)
	}
}
