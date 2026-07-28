package node

import (
	"os"
	"testing"

	"github.com/geodro/lerd/internal/hostbin"
)

// The Node resolution tests build a whole host layout under a temp HOME and a
// PATH of their own. The real install prefixes (/opt/homebrew/bin on macOS)
// would let whatever the developer's machine has installed decide the result,
// so they are empty by default and each test that needs them opts in.
func TestMain(m *testing.M) {
	hostbin.ExtraDirs = func() []string { return nil }
	os.Exit(m.Run())
}
