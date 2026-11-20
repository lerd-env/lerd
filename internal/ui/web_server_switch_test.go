package ui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// The dashboard switches the web server through the same code as the CLI,
// and gets the switch's own words back, a refusal included.
func TestWebServerSwitch_runsTheCLISwitch(t *testing.T) {
	var got string
	prev := webServerSwitchFn
	t.Cleanup(func() { webServerSwitchFn = prev })
	webServerSwitchFn = func(kind string, out io.Writer) error {
		got = kind
		io.WriteString(out, "serving with "+kind+"\n")
		if kind == "go" {
			return nil
		}
		return errors.New("refused for a reason")
	}
	post := func(body string) webServerSwitchResponse {
		rec := httptest.NewRecorder()
		handleWebServerSwitch(rec, httptest.NewRequest("POST", "/api/web-server", strings.NewReader(body)))
		var res webServerSwitchResponse
		json.Unmarshal(rec.Body.Bytes(), &res)
		return res
	}
	if res := post(`{"kind":"go"}`); !res.OK || got != "go" || !strings.Contains(res.Output, "serving with go") {
		t.Fatalf("switch: %+v (ran %q)", res, got)
	}
	if res := post(`{"kind":"nginx"}`); res.OK || res.Error != "refused for a reason" {
		t.Fatalf("refusal: %+v", res)
	}
	rec := httptest.NewRecorder()
	handleWebServerSwitch(rec, httptest.NewRequest("GET", "/api/web-server", nil))
	if rec.Code != 405 {
		t.Errorf("GET %d, want 405", rec.Code)
	}
}

// The status every client reads is cached; a switch, done or refused, has to
// drop it, or the dashboard keeps showing the server that no longer runs and
// offers to switch to the one already serving.
func TestWebServerSwitch_refreshesTheStatus(t *testing.T) {
	published := 0
	prevFn, prevAfter := webServerSwitchFn, afterWebServerSwitch
	t.Cleanup(func() { webServerSwitchFn, afterWebServerSwitch = prevFn, prevAfter })
	webServerSwitchFn = func(string, io.Writer) error { return nil }
	afterWebServerSwitch = func() { published++ }
	handleWebServerSwitch(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/web-server", strings.NewReader(`{"kind":"go"}`)))
	webServerSwitchFn = func(string, io.Writer) error { return errors.New("no") }
	handleWebServerSwitch(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/web-server", strings.NewReader(`{"kind":"go"}`)))
	if published != 2 {
		t.Fatalf("status published %d times, want 2", published)
	}
}
