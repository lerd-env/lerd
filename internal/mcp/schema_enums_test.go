package mcp

import (
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/dumps"
)

func propEnum(t *testing.T, tool, prop string) []string {
	t.Helper()
	for _, tl := range toolList() {
		if tl.Name == tool {
			return tl.InputSchema.Properties[prop].Enum
		}
	}
	t.Fatalf("tool %q is not registered", tool)
	return nil
}

// A client that validates against the schema never sends a value the enum
// leaves out, so a kind the Debug window gained is unreachable until it is here.
func TestDumpsKindEnumCoversEveryKind(t *testing.T) {
	enum := propEnum(t, "diag", "kind")
	for _, k := range []string{dumps.KindDump, dumps.KindQuery, dumps.KindJob, dumps.KindView,
		dumps.KindMail, dumps.KindCache, dumps.KindEvent, dumps.KindHTTP,
		dumps.KindLog, dumps.KindException, dumps.KindMessage} {
		if !slices.Contains(enum, k) {
			t.Errorf("diag kind enum is missing %q", k)
		}
	}
}

// mise is the default manager, so an assistant has to be able to switch back to it.
func TestNodeManagerEnumOffersMise(t *testing.T) {
	if !slices.Contains(propEnum(t, "runtime", "manager"), "mise") {
		t.Error("runtime manager enum is missing mise")
	}
}
