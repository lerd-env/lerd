package ui

import "testing"

func TestNotifyOnPHPBaseUpdates_FirstSnapshotIsQuiet(t *testing.T) {
	phpUpdatesPrev = nil
	got := phpBaseUpdateTransitions([]byte(`{"php_fpms":[{"version":"8.4","update_available":true}]}`))
	if len(got) != 0 {
		t.Errorf("the first snapshot has nothing to compare against, got %v", got)
	}
	if !phpUpdatesPrev["8.4"] {
		t.Error("the first snapshot must still seed the previous state")
	}
}

func TestNotifyOnPHPBaseUpdates_FiresOnTransitionOnly(t *testing.T) {
	phpUpdatesPrev = map[string]bool{"8.3": false, "8.4": true}
	got := phpBaseUpdateTransitions([]byte(
		`{"php_fpms":[{"version":"8.3","update_available":true},{"version":"8.4","update_available":true}]}`))
	if len(got) != 1 || got[0] != "8.3" {
		t.Errorf("expected only the newly-stale version, got %v", got)
	}

	// A rebuild clears the flag; going quiet is not a notification either.
	phpUpdatesPrev = map[string]bool{"8.3": true}
	if got := phpBaseUpdateTransitions([]byte(`{"php_fpms":[{"version":"8.3"}]}`)); len(got) != 0 {
		t.Errorf("a cleared flag must not notify, got %v", got)
	}
}

func TestNotificationForPHPBaseUpdate_Shape(t *testing.T) {
	n := notificationForPHPBaseUpdate("8.4")
	if n.Kind != "update_available" {
		t.Errorf("Kind = %q", n.Kind)
	}
	if n.URL != "#system/php-8.4" {
		t.Errorf("URL = %q", n.URL)
	}
	if n.Params["service"] != "PHP 8.4" {
		t.Errorf("Params.service = %q", n.Params["service"])
	}
}
