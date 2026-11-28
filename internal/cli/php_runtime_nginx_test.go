package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubWebServerMove records what moving nginx asks for, in order, so no test
// rewrites or restarts the developer's own nginx.
func stubWebServerMove(t *testing.T, nowOnHost bool) *[]string {
	t.Helper()
	var calls []string
	prevOn, prevRest, prevUnit, prevRestart, prevImage := moveOnHostFn, moveRewriteRestFn, moveRewriteUnitFn, moveRestartUnitFn, moveEnsureImageFn
	t.Cleanup(func() {
		moveOnHostFn, moveRewriteRestFn, moveRewriteUnitFn, moveRestartUnitFn, moveEnsureImageFn = prevOn, prevRest, prevUnit, prevRestart, prevImage
	})
	moveOnHostFn = func() bool { return nowOnHost }
	moveRewriteRestFn = func([]config.Site) { calls = append(calls, "configs") }
	moveRewriteUnitFn = func() (bool, error) { calls = append(calls, "unit"); return true, nil }
	moveRestartUnitFn = func() error { calls = append(calls, "restart"); return nil }
	moveEnsureImageFn = func() { calls = append(calls, "image") }
	return &calls
}

// Switching PHP to the host takes nginx with it: every config is rewritten for
// the host, the unit swapped, and nginx restarted on it rather than reloaded.
func TestMoveWebServer_followsPHPToTheHost(t *testing.T) {
	calls := stubWebServerMove(t, true)
	if !moveWebServer(false, nil) {
		t.Fatal("nginx did not move with PHP")
	}
	if got := strings.Join(*calls, ","); got != "configs,unit,restart" {
		t.Errorf("move = %s, want configs,unit,restart", got)
	}
}

// Coming back, the container needs its image before it can start: moving to
// the host offers to remove images, and nginx's may be among them.
func TestMoveWebServer_bringsTheContainerBack(t *testing.T) {
	calls := stubWebServerMove(t, false)
	if !moveWebServer(true, nil) {
		t.Fatal("nginx did not move back")
	}
	if got := strings.Join(*calls, ","); got != "configs,image,unit,restart" {
		t.Errorf("move = %s, want configs,image,unit,restart", got)
	}
}

func TestMoveWebServer_staysPutWhenItsHomeDidNotChange(t *testing.T) {
	calls := stubWebServerMove(t, true)
	if moveWebServer(true, nil) || len(*calls) != 0 {
		t.Errorf("moved without a change of home: %v", *calls)
	}
}
