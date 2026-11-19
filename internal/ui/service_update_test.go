package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/serviceops"
)

func stubServiceUpdate(t *testing.T, available bool) *int {
	t.Helper()
	calls := 0
	prevCheck, prevRefresh := checkServiceUpdateFn, refreshServiceUpdateFn
	checkServiceUpdateFn = func(name string) (*serviceops.UpdateAvailability, error) {
		calls++
		return &serviceops.UpdateAvailability{Service: name, Available: available}, nil
	}
	refreshServiceUpdateFn = checkServiceUpdateFn
	t.Cleanup(func() {
		checkServiceUpdateFn, refreshServiceUpdateFn = prevCheck, prevRefresh
		updates.dropFor("")
	})
	updates.dropFor("")
	return &calls
}

func TestServiceUpdate_ReadOncePerStatusWindow(t *testing.T) {
	calls := stubServiceUpdate(t, true)
	serviceUpdate("redis")
	serviceUpdate("redis")
	if *calls != 1 {
		t.Errorf("checked redis %d times across two rebuilds, want 1", *calls)
	}
}

// Check for updates must replace the stored answer, or the button reports one
// thing and the card keeps showing the other for up to an hour.
func TestServiceUpdate_ManualCheckReplacesTheAnswer(t *testing.T) {
	stubServiceUpdate(t, false)
	if serviceUpdate("redis").Available {
		t.Fatal("stub should start with no update")
	}
	checkServiceUpdateFn = func(name string) (*serviceops.UpdateAvailability, error) {
		return &serviceops.UpdateAvailability{Service: name, Available: true}, nil
	}
	refreshServiceUpdateFn = checkServiceUpdateFn
	req := httptest.NewRequest(http.MethodPost, "/api/services/redis/updates", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	handleServiceAction(httptest.NewRecorder(), req)

	if !serviceUpdate("redis").Available {
		t.Error("the card still shows the answer from before the manual check")
	}
}
