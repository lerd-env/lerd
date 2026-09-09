package tools

import (
	"os"
	"testing"
)

// The native PHP pins come from a second manifest fetched at runtime, so
// without this every test that loads the manifest reaches the published one
// and starts depending on how many builds happen to be released. Pointed at an
// address nothing answers on; the tests that are about those pins set their own
// server and override this.
func TestMain(m *testing.M) {
	os.Setenv("LERD_NATIVE_PHP_URL", "http://127.0.0.1:1/native-php.yaml") //nolint:errcheck
	os.Exit(m.Run())
}
