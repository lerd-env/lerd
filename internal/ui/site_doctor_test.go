package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// TestDoctorRoute_unknownBranchRefused: a branch that doesn't resolve to a
// worktree must not fall back to the parent checkout, or the doctor would
// silently diagnose the main site's .env and database instead of the worktree.
func TestDoctorRoute_unknownBranchRefused(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}, Framework: "laravel"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/doctor?branch=ghost", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	if !doctorRoute(rec, req, "acme.test", []string{"doctor"}) {
		t.Fatal("doctorRoute did not handle the request")
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("unknown branch: expected an error, got %s", rec.Body.String())
	}
	if _, ok := resp["checks"]; ok {
		t.Error("unknown branch must not return checks (would be the parent's)")
	}
}

// The panel draws each finding as it lands and takes its totals from the final
// frame, so every check goes out as its own event before a done carrying all.
func TestStreamDoctor_sendsEachCheckThenTheReport(t *testing.T) {
	rec := httptest.NewRecorder()
	streamDoctor(rec, func(onCheck func(sitedoctor.Check)) sitedoctor.Response {
		a := sitedoctor.Check{Name: "a", Status: sitedoctor.StatusOK}
		b := sitedoctor.Check{Name: "b", Status: sitedoctor.StatusFail}
		onCheck(a)
		onCheck(b)
		return sitedoctor.Response{Checks: []sitedoctor.Check{a, b}, Failures: 1}
	})

	body := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type = %q, want text/event-stream", ct)
	}
	if n := strings.Count(body, "event: check\n"); n != 2 {
		t.Errorf("got %d check events, want 2:\n%s", n, body)
	}
	done := body[strings.Index(body, "event: done\ndata: ")+len("event: done\ndata: "):]
	var resp sitedoctor.Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(done)), &resp); err != nil {
		t.Fatalf("done frame: %v\n%s", err, body)
	}
	if resp.Failures != 1 || len(resp.Checks) != 2 {
		t.Errorf("done report = %+v, want both checks and one failure", resp)
	}
	if strings.Index(body, "event: done") < strings.LastIndex(body, "event: check") {
		t.Error("done was sent before the last check")
	}
}

func TestDoctorFixRun_RejectsUnknownKey(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/doctor/fix/rm-rf/run", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if !strings.Contains(rec.Body.String(), "unknown doctor fix") {
		t.Errorf("expected unknown-fix error, got %q", rec.Body.String())
	}
}

func TestDoctorFixRun_StreamsAllowlistedCommand(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/doctor/fix/composer_install/run", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	// composer isn't on PATH in the test env, so the run exits non-zero, but it
	// must still stream a done frame rather than erroring out the endpoint.
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Errorf("expected a done event from the streamed fix, got %q", rec.Body.String())
	}
}

// Creating a missing schema is a host action rather than a container command,
// so it has to be routed before the allowlist lookup that would reject it.
func TestDoctorFixRun_CreateDatabaseIsAHostAction(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/doctor/fix/database_create/run", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if strings.Contains(rec.Body.String(), "unknown doctor fix") {
		t.Fatalf("the create-database fix should be handled, got %q", rec.Body.String())
	}
	// The registered site points at no database, so nothing is created; it must
	// still say so through a done frame rather than erroring out the endpoint.
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Errorf("expected a done event, got %q", rec.Body.String())
	}
}

// Creating a missing bucket runs against the service rather than the site's
// container, so it is routed the same way the schema one is.
func TestDoctorFixRun_CreateBucketIsAHostAction(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/doctor/fix/bucket_create/run", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if strings.Contains(rec.Body.String(), "unknown doctor fix") {
		t.Fatalf("the create-bucket fix should be handled, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Errorf("expected a done event, got %q", rec.Body.String())
	}
}

func TestDoctorFixRun_NonLoopbackForbidden(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	req := httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/doctor/fix/composer_install/run", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("non-loopback fix should be forbidden, got %d", rec.Code)
	}
}
