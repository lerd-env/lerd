package cli

import (
	"reflect"
	"testing"
)

// A start that hands back control while an engine is still booting makes the
// user's first page load fail, so the service names have to come back off the
// unit list to be waited on.
func TestServiceNamesForReadiness(t *testing.T) {
	units := []string{"lerd-mysql", "lerd-redis", "lerd-mailpit"}
	want := []string{"mysql", "redis", "mailpit"}
	if got := serviceNamesForReadiness(units); !reflect.DeepEqual(got, want) {
		t.Errorf("serviceNamesForReadiness(%v) = %v, want %v", units, got, want)
	}
}

// The UI, the watcher and timer units are not engines and have no probe, so
// waiting on them would only add latency to every start.
func TestServiceNamesForReadinessSkipsNonEngines(t *testing.T) {
	units := []string{"lerd-ui", "lerd-watcher", "lerd-postgres", "lerd-cleanup.timer"}
	want := []string{"postgres"}
	if got := serviceNamesForReadiness(units); !reflect.DeepEqual(got, want) {
		t.Errorf("serviceNamesForReadiness(%v) = %v, want %v", units, got, want)
	}
}
