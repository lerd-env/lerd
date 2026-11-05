package ui

import (
	"slices"
	"testing"
)

func TestExternalDBWatch_NotifiesOnceWhenAServerGoesAway(t *testing.T) {
	var w externalDBWatch
	if down, changed := w.observe(map[string]bool{"a": true, "b": false}); len(down) != 0 || changed {
		t.Fatalf("first look must stay quiet: %v %v", down, changed)
	}
	down, changed := w.observe(map[string]bool{"a": false, "b": false})
	if !slices.Equal(down, []string{"a"}) || !changed {
		t.Fatalf("got %v %v", down, changed)
	}
	if down, changed := w.observe(map[string]bool{"a": false, "b": false}); len(down) != 0 || changed {
		t.Fatalf("still down must not notify again: %v %v", down, changed)
	}
}

// Coming back has to refresh the page, though nobody needs telling.
func TestExternalDBWatch_RefreshesWhenAServerReturns(t *testing.T) {
	w := externalDBWatch{up: map[string]bool{"a": false}}
	down, changed := w.observe(map[string]bool{"a": true})
	if len(down) != 0 || !changed {
		t.Fatalf("got %v %v", down, changed)
	}
}

func TestExternalDBWatch_AServerAddedLaterIsNotANewFailure(t *testing.T) {
	w := externalDBWatch{up: map[string]bool{}}
	if down, _ := w.observe(map[string]bool{"new": false}); len(down) != 0 {
		t.Fatalf("got %v", down)
	}
}

func TestNotificationForExternalDBOffline(t *testing.T) {
	n := notificationForExternalDBOffline("dbngin", "host.containers.internal:3306")
	if n.TitleKey != "notify_external_db_offline_title" || n.Params["name"] != "dbngin" || n.URL != "#services/dbngin" {
		t.Errorf("got %+v", n)
	}
}
