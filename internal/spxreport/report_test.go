package spxreport

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A trace where main calls a slow function then a fast one, wall in nanoseconds.
// Exclusive: slow 1.0ms, main 0.3ms, fast 0.1ms; total 1.4ms.
const sampleTrace = `[events]
0 1 0 0
1 1 100000 0
1 0 1100000 0
2 1 1200000 0
2 0 1300000 0
0 0 1400000 0
[functions]
main
slow
fast
`

func TestParseFlat(t *testing.T) {
	prof, err := parseFlat(strings.NewReader(sampleTrace), 8, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if prof.WallMS != 1.4 {
		t.Errorf("wall = %v, want 1.4", prof.WallMS)
	}
	if len(prof.Hotspots) != 3 {
		t.Fatalf("hotspots = %d, want 3", len(prof.Hotspots))
	}
	top := prof.Hotspots[0]
	if top.Function != "slow" {
		t.Errorf("top function = %q, want slow", top.Function)
	}
	if top.ExclMS != 1.0 {
		t.Errorf("top excl = %v, want 1.0", top.ExclMS)
	}
	if top.Pct != 71.4 {
		t.Errorf("top pct = %v, want 71.4", top.Pct)
	}
	if top.Calls != 1 {
		t.Errorf("top calls = %v, want 1", top.Calls)
	}
}

func TestParseFlat_ThresholdKeepsWorstAndCapsTail(t *testing.T) {
	// minPct high enough that only the biggest survives, but it must still appear.
	prof, _ := parseFlat(strings.NewReader(sampleTrace), 8, 50.0)
	if len(prof.Hotspots) != 1 || prof.Hotspots[0].Function != "slow" {
		t.Fatalf("want only the worst offender, got %+v", prof.Hotspots)
	}
}

func writeReport(t *testing.T, dir, key, meta string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(sampleTrace))
	gw.Close()
	if err := os.WriteFile(filepath.Join(dir, key+".txt.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProfilesForRoutes(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "old", `{"exec_ts":1000,"http_method":"GET","http_host":"acme.test","http_request_uri":"/users/5","cli":0}`)
	writeReport(t, dir, "new", `{"exec_ts":2000,"http_method":"GET","http_host":"acme.test","http_request_uri":"/users/9","cli":0}`)
	writeReport(t, dir, "otherhost", `{"exec_ts":3000,"http_method":"GET","http_host":"evil.test","http_request_uri":"/users/1","cli":0}`)
	writeReport(t, dir, "cli", `{"exec_ts":4000,"http_method":"","http_host":"","http_request_uri":"","cli":1}`)

	got := ProfilesForRoutes(dir, []string{"acme.test"}, []string{"GET /users/:id"}, 8, 1.0)
	p, ok := got["GET /users/:id"]
	if !ok {
		t.Fatal("expected a profile for GET /users/:id")
	}
	// Freshest (exec_ts 2000, /users/9) wins over the older /users/5 capture.
	if p.URI != "/users/9" {
		t.Errorf("uri = %q, want the freshest /users/9", p.URI)
	}
	if len(p.Hotspots) == 0 || p.Hotspots[0].Function != "slow" {
		t.Errorf("hotspots not parsed: %+v", p.Hotspots)
	}

	// A route with no capture, and a host we don't own, both yield nothing.
	if _, ok := ProfilesForRoutes(dir, []string{"acme.test"}, []string{"POST /checkout"}, 8, 1.0)["POST /checkout"]; ok {
		t.Error("a route with no capture must not appear")
	}
	if len(ProfilesForRoutes(dir, []string{"nope.test"}, []string{"GET /users/:id"}, 8, 1.0)) != 0 {
		t.Error("captures for a host we did not ask for must be ignored")
	}
}
