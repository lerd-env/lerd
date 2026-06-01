package ui

import (
	"encoding/json"
	"testing"

	"github.com/geodro/lerd/internal/dumps"
)

func aqEvent(rid, sql string, ms float64, file string, line int) dumps.Event {
	d, _ := json.Marshal(dumps.QueryData{SQL: sql, TimeMS: ms})
	return dumps.Event{
		Kind: dumps.KindQuery,
		Ctx:  dumps.Context{Type: "fpm", Site: "acme", Request: "GET /users", RID: rid},
		Src:  dumps.Source{File: file, Line: line},
		Data: d,
	}
}

func TestAnalyzeQueries_DetectsNPlusOne(t *testing.T) {
	// Same query shape (different literals) three times in one request.
	events := []dumps.Event{
		aqEvent("r1", "select * from posts where user_id = 1", 2, "/app/User.php", 30),
		aqEvent("r1", "select * from posts where user_id = 2", 3, "/app/User.php", 30),
		aqEvent("r1", "select * from posts where user_id = 3", 2, "/app/User.php", 30),
	}
	rep := analyzeQueries(events, 0, 0) // defaults: minRepeat 3, slow 100
	if len(rep.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(rep.Requests))
	}
	req := rep.Requests[0]
	if len(req.NPlusOne) != 1 {
		t.Fatalf("n_plus_one = %d, want 1", len(req.NPlusOne))
	}
	f := req.NPlusOne[0]
	if f.Count != 3 {
		t.Errorf("count = %d, want 3", f.Count)
	}
	if f.Fingerprint != "select * from posts where user_id = ?" {
		t.Errorf("fingerprint = %q", f.Fingerprint)
	}
	if f.Caller.File != "/app/User.php" || f.Caller.Line != 30 {
		t.Errorf("caller = %+v, want /app/User.php:30", f.Caller)
	}
	if rep.Summary.NPlusOneFindings != 1 || rep.Summary.RequestsAnalyzed != 1 {
		t.Errorf("summary = %+v", rep.Summary)
	}
}

func TestAnalyzeQueries_BelowThresholdExcluded(t *testing.T) {
	events := []dumps.Event{
		aqEvent("r1", "select * from posts where user_id = 1", 2, "/app/User.php", 30),
		aqEvent("r1", "select * from posts where user_id = 2", 2, "/app/User.php", 30),
	}
	rep := analyzeQueries(events, 0, 0)
	if len(rep.Requests) != 0 {
		t.Fatalf("two repeats should not trip the default-3 threshold: %+v", rep.Requests)
	}
}

func TestAnalyzeQueries_DetectsSlow(t *testing.T) {
	events := []dumps.Event{
		aqEvent("r1", "select * from big_report", 210, "/app/Report.php", 12),
	}
	rep := analyzeQueries(events, 0, 0)
	if len(rep.Requests) != 1 || len(rep.Requests[0].Slow) != 1 {
		t.Fatalf("want one slow finding, got %+v", rep.Requests)
	}
	if rep.Requests[0].Slow[0].TimeMS != 210 {
		t.Errorf("slow time = %v", rep.Requests[0].Slow[0].TimeMS)
	}
}

func TestAnalyzeQueries_GroupsByRequest(t *testing.T) {
	mk := func(rid string) []dumps.Event {
		return []dumps.Event{
			aqEvent(rid, "select * from t where id = 1", 1, "/a.php", 1),
			aqEvent(rid, "select * from t where id = 2", 1, "/a.php", 1),
			aqEvent(rid, "select * from t where id = 3", 1, "/a.php", 1),
		}
	}
	events := append(mk("r1"), mk("r2")...)
	rep := analyzeQueries(events, 3, 100)
	if len(rep.Requests) != 2 {
		t.Fatalf("requests = %d, want 2 (one per rid)", len(rep.Requests))
	}
}

func TestAnalyzeQueries_NonQueryAndEmptyIgnored(t *testing.T) {
	rep := analyzeQueries(nil, 0, 0)
	if len(rep.Requests) != 0 || rep.Requests == nil {
		t.Fatalf("nil input should yield an empty (non-nil) request list: %+v", rep)
	}
}
