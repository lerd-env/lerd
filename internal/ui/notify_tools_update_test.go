package ui

import "testing"

func TestNotifyOnToolUpdates_FirstSnapshotIsQuiet(t *testing.T) {
	toolUpdatesPrev = nil
	got := toolUpdateTransitions([]byte(`{"tools":[{"name":"composer","present":true,"update_available":true}]}`))
	if len(got) != 0 {
		t.Errorf("the first snapshot has nothing to compare against, got %v", got)
	}
	if !toolUpdatesPrev["composer"] {
		t.Error("the first snapshot must still seed the previous state")
	}
}

func TestNotifyOnToolUpdates_FiresOnTransitionOnly(t *testing.T) {
	toolUpdatesPrev = map[string]bool{"composer": false, "mkcert": true}
	got := toolUpdateTransitions([]byte(
		`{"tools":[{"name":"composer","pinned":"2.10.3","present":true,"update_available":true},` +
			`{"name":"mkcert","pinned":"v1.4.4","present":true,"update_available":true}]}`))
	if len(got) != 1 || got[0].name != "composer" {
		t.Fatalf("expected only the newly-pending tool, got %+v", got)
	}
	if got[0].version != "2.10.3" {
		t.Errorf("version = %q, want the pinned version to announce", got[0].version)
	}

	// Applying the update clears the flag; going quiet is not a notification.
	toolUpdatesPrev = map[string]bool{"composer": true}
	if got := toolUpdateTransitions([]byte(`{"tools":[{"name":"composer","present":true}]}`)); len(got) != 0 {
		t.Errorf("a cleared flag must not notify, got %+v", got)
	}
}

// A tool that is not installed at all has no pending update to announce: the
// pin is the version an install would bring, not one the user is behind on.
func TestNotifyOnToolUpdates_IgnoresAMissingTool(t *testing.T) {
	toolUpdatesPrev = map[string]bool{"fnm": false}
	got := toolUpdateTransitions([]byte(`{"tools":[{"name":"fnm","present":false,"update_available":true}]}`))
	if len(got) != 0 {
		t.Errorf("a tool that is not installed must not notify, got %+v", got)
	}
}

func TestNotificationForToolUpdate_Shape(t *testing.T) {
	n := notificationForToolUpdate("composer", "2.10.3")
	if n.Kind != "update_available" {
		t.Errorf("Kind = %q", n.Kind)
	}
	if n.URL != "#system/tools" {
		t.Errorf("URL = %q, want the card that applies it", n.URL)
	}
	if n.Params["service"] != "composer" || n.Params["version"] != "2.10.3" {
		t.Errorf("Params = %v", n.Params)
	}
	// Two tools going stale in the same snapshot must not collapse onto one
	// notification, the way two services do not.
	if other := notificationForToolUpdate("mkcert", "v1.4.4"); other.Tag == n.Tag {
		t.Errorf("both tools share the tag %q", n.Tag)
	}
}
