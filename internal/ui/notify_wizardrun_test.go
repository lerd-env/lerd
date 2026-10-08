package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/push"
)

// stubRunNotifier hands over what a finished run would raise. The run's own
// goroutine sends, so a channel rather than a slice keeps the test race-free.
func stubRunNotifier(t *testing.T) chan push.Notification {
	t.Helper()
	sent := make(chan push.Notification, 4)
	original := dispatchRunNotification
	dispatchRunNotification = func(n push.Notification) { sent <- n }
	t.Cleanup(func() { dispatchRunNotification = original })
	return sent
}

func receiveNotification(t *testing.T, sent chan push.Notification) push.Notification {
	t.Helper()
	select {
	case n := <-sent:
		return n
	case <-time.After(time.Second):
		t.Fatal("the run raised no notification")
		return push.Notification{}
	}
}

// Scaffolding is the run the wizard exists to send to the background, so its
// ending has to reach the user with no dashboard page open.
func TestNotificationForScaffoldRun(t *testing.T) {
	n, ok := notificationForRun(runSnapshot{
		Kind:   runKindScaffold,
		Dir:    "/home/u/projects",
		Label:  "/home/u/projects/shop",
		Status: runDone,
	}, time.Now())
	if !ok {
		t.Fatal("a finished scaffold should notify")
	}
	if n.Kind != "op_done" {
		t.Errorf("kind = %q, want op_done", n.Kind)
	}
	if !strings.Contains(n.Title, "shop") {
		t.Errorf("title = %q, want it to name the project", n.Title)
	}
}

// A step that broke is worth interrupting for whichever kind of run it was.
func TestNotificationForFailedRun(t *testing.T) {
	n, ok := notificationForRun(runSnapshot{
		Kind:   runKindSetup,
		Dir:    "/home/u/projects/shop",
		Status: runFailed,
		Error:  "composer could not resolve dependencies",
	}, time.Now())
	if !ok {
		t.Fatal("a failed run should notify")
	}
	if n.Kind != "op_failed" {
		t.Errorf("kind = %q, want op_failed", n.Kind)
	}
	if !strings.Contains(n.Body, "resolve dependencies") {
		t.Errorf("body = %q, want the reason it failed", n.Body)
	}
	if !strings.Contains(n.Title, "shop") {
		t.Errorf("title = %q, want it to name the project", n.Title)
	}
}

// A full setup is five or six quick steps. One notification each would bury the
// ones that matter, and the wizard is already showing them on screen.
func TestQuickRunsStayQuiet(t *testing.T) {
	for _, kind := range []string{runKindLink, runKindEnv, runKindSetup} {
		if _, ok := notificationForRun(runSnapshot{Kind: kind, Dir: "/home/u/shop", Status: runDone}, time.Now()); ok {
			t.Errorf("a finished %s run should not notify", kind)
		}
	}
}

// The registry raises it itself, so nothing depends on a page still watching.
func TestRunRegistryNotifiesOnFailure(t *testing.T) {
	sent := stubRunNotifier(t)
	stubRunExec(t, func(r *run) error {
		r.append("could not write to disk")
		return errors.New("exit status 1")
	})

	reg := newRunRegistry()
	r := reg.Start(runKindScaffold, t.TempDir(), "/tmp/shop", []string{"lerd", "new"})
	waitForStatus(t, r, runFailed)

	if n := receiveNotification(t, sent); n.Kind != "op_failed" {
		t.Errorf("kind = %q, want op_failed", n.Kind)
	}
	if len(sent) != 0 {
		t.Errorf("raised %d extra notifications, want 1 in all", len(sent))
	}
}

// A finished scaffold reports itself the same way, so a project created while
// the dashboard was closed is not silently waiting.
func TestRunRegistryNotifiesOnScaffoldSuccess(t *testing.T) {
	sent := stubRunNotifier(t)
	stubRunExec(t, func(_ *run) error { return nil })

	reg := newRunRegistry()
	r := reg.Start(runKindScaffold, t.TempDir(), "/tmp/shop", []string{"lerd", "new"})
	waitForStatus(t, r, runDone)

	if n := receiveNotification(t, sent); n.Kind != "op_done" {
		t.Errorf("kind = %q, want op_done", n.Kind)
	}
}
