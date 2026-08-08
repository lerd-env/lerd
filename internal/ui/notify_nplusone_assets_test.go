package ui

import (
	"encoding/json"
	"testing"

	"github.com/geodro/lerd/internal/dumps"
)

func assetQueryEvent(request string) dumps.Event {
	data, _ := json.Marshal(dumps.QueryData{SQL: "SELECT * FROM cachetags WHERE tag = 'x'"})
	return dumps.Event{
		V:    1,
		Kind: dumps.KindQuery,
		Data: data,
		Ctx:  dumps.Context{RID: "r1", Site: "example.test", Request: request},
	}
}

// Drupal builds its aggregated CSS and JS through PHP, so serving one runs
// queries, and each aggregate carries its own hash: every request looks like a
// route the tracker has never warned about, and the warnings arrive one per
// asset until the user turns them off.
func TestNPlusOne_ignoresStaticAssets(t *testing.T) {
	tracker := newNPlusOneTracker()
	req := "GET /sites/default/files/js/js_yuCohp6uawaNteNfojZh1mhguIFEPb3zGDEk-jTLPQE.js?scope=footer&delta=1"
	for range 10 {
		if n := tracker.observe(assetQueryEvent(req)); n != nil {
			t.Fatalf("a static asset produced a warning: %s", n.Body)
		}
	}
}

// An application route still warns, so the filter takes nothing real away.
func TestNPlusOne_stillWarnsOnAnAppRoute(t *testing.T) {
	tracker := newNPlusOneTracker()
	var got bool
	for range 5 {
		if n := tracker.observe(assetQueryEvent("GET /admin/content")); n != nil {
			got = true
		}
	}
	if !got {
		t.Error("an app route stopped warning")
	}
}

// A run with no request at all (a worker or a console command) is unaffected.
func TestNPlusOne_workerStillWarns(t *testing.T) {
	tracker := newNPlusOneTracker()
	ev := assetQueryEvent("")
	ev.Ctx.Worker = "queue"
	var got bool
	for range 5 {
		if n := tracker.observe(ev); n != nil {
			got = true
		}
	}
	if !got {
		t.Error("a worker stopped warning")
	}
}
