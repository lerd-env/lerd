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
	cachedToolStatus("fnm")
	cachedToolStatus("fnm")
	if *calls != 1 {
		t.Errorf("read tools %d times across two rebuilds, want 1", *calls)
	}
}

func TestToolStatus_ManualCheckReplacesTheAnswer(t *testing.T) {
	stubToolStatus(t, false)
	cachedToolStatus("fnm")
	toolStatusFn = func(context.Context) []tools.ToolStatus {
		return []tools.ToolStatus{{Name: "composer", Present: true, UpdateAvailable: true}}
	}
	handleTools(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/tools/check", nil))

	if got := cachedToolStatus("fnm"); !got[0].UpdateAvailable {
		t.Error("composer still shows the answer from before the manual check")
	}
}

// The list carries the active Node manager's row, so switching managers must
// not serve the list worked out for the other one.
func TestToolStatus_SwitchingNodeManagerRereads(t *testing.T) {
	calls := stubToolStatus(t, false)
	cachedToolStatus("fnm")
	cachedToolStatus("nvm")
	cachedToolStatus("fnm")
	if *calls != 2 {
		t.Errorf("read tools %d times across fnm, nvm, fnm, want 2", *calls)
	}
}
