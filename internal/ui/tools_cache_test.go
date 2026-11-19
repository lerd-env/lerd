package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/tools"
)

func stubToolStatus(t *testing.T, update bool) *int {
	t.Helper()
	calls := 0
	prevStatus, prevRefresh := toolStatusFn, refreshPinsFn
	toolStatusFn = func(context.Context) []tools.ToolStatus {
		calls++
		return []tools.ToolStatus{{Name: "composer", Present: true, UpdateAvailable: update}}
	}
	refreshPinsFn = func(context.Context) *tools.Manifest { return nil }
	t.Cleanup(func() {
		toolStatusFn, refreshPinsFn = prevStatus, prevRefresh
		updates.dropFor("")
	})
	updates.dropFor("")
	return &calls
}

func TestToolStatus_ReadOncePerStatusWindow(t *testing.T) {
	calls := stubToolStatus(t, false)
	cachedToolStatus()
	cachedToolStatus()
	if *calls != 1 {
		t.Errorf("read tools %d times across two rebuilds, want 1", *calls)
	}
}

func TestToolStatus_ManualCheckReplacesTheAnswer(t *testing.T) {
	stubToolStatus(t, false)
	cachedToolStatus()
	toolStatusFn = func(context.Context) []tools.ToolStatus {
		return []tools.ToolStatus{{Name: "composer", Present: true, UpdateAvailable: true}}
	}
	handleTools(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/tools/check", nil))

	if got := cachedToolStatus(); !got[0].UpdateAvailable {
		t.Error("composer still shows the answer from before the manual check")
	}
}
