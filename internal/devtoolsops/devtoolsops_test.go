package devtoolsops

import (
	"os"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// isolate pins HOME and the XDG dirs to temp dirs so the toggle writes its
// sentinel flags, mounted ini, and config.yaml into a sandbox rather than the
// real environment. Apply/SetWorkers are pure filesystem ops (no container
// calls), so this is enough to exercise them end to end.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestSetWorkersTogglesFlag(t *testing.T) {
	isolate(t)

	res, err := SetWorkers(true)
	if err != nil {
		t.Fatalf("enable workers: %v", err)
	}
	if !res.Workers || res.NoChange {
		t.Fatalf("enable result = %+v, want {Workers:true NoChange:false}", res)
	}
	if !fileExists(config.DevtoolsWorkersFlagFile()) {
		t.Fatal("workers sentinel missing after enable")
	}

	res, err = SetWorkers(true)
	if err != nil {
		t.Fatalf("re-enable workers: %v", err)
	}
	if !res.NoChange {
		t.Fatalf("re-enable result = %+v, want NoChange:true", res)
	}

	res, err = SetWorkers(false)
	if err != nil {
		t.Fatalf("disable workers: %v", err)
	}
	if res.Workers {
		t.Fatalf("disable result = %+v, want Workers:false", res)
	}
	if fileExists(config.DevtoolsWorkersFlagFile()) {
		t.Fatal("workers sentinel still present after disable")
	}
}
