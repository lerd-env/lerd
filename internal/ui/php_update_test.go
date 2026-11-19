package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func stubPHPState(t *testing.T, update bool) *int {
	t.Helper()
	calls := 0
	prevState, prevRefresh := phpStateFn, refreshPHPBaseFn
	phpStateFn = func(string, bool) phpImageState { calls++; return phpImageState{Patch: "8.4.1", Update: update} }
	refreshPHPBaseFn = func(string) any { return nil }
	t.Cleanup(func() {
		phpStateFn, refreshPHPBaseFn = prevState, prevRefresh
		updates.dropFor("")
	})
	updates.dropFor("")
	return &calls
}

func TestPHPState_ReadOncePerStatusWindow(t *testing.T) {
	calls := stubPHPState(t, false)
	phpState("8.4", false)
	phpState("8.4", false)
	if *calls != 1 {
		t.Errorf("read PHP 8.4 %d times across two rebuilds, want 1", *calls)
	}
}

func TestPHPState_ManualCheckReplacesTheAnswer(t *testing.T) {
	stubPHPState(t, false)
	phpState("8.4", false)
	phpStateFn = func(string, bool) phpImageState { return phpImageState{Patch: "8.4.1", Update: true} }

	req := httptest.NewRequest(http.MethodPost, "/api/php-versions/8.4/updates", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	handlePHPVersionAction(httptest.NewRecorder(), req)

	if !phpState("8.4", false).Update {
		t.Error("PHP 8.4 still shows the answer from before the manual check")
	}
}
