package reqstats

import "testing"

func TestParseAccessRecord(t *testing.T) {
	// Full syslog framing nginx wraps the pipe-delimited message in.
	dg := []byte("<190>Jul  2 10:00:00 lerdaccess: myapp.test|200|0.042|GET|/reports/5?p=2")
	r, ok := ParseAccessRecord(dg)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if r.Host != "myapp.test" {
		t.Errorf("host = %q", r.Host)
	}
	if r.Status != 200 {
		t.Errorf("status = %d", r.Status)
	}
	if r.SecondsToMillis() != 42 {
		t.Errorf("millis = %v", r.SecondsToMillis())
	}
	if r.Method != "GET" {
		t.Errorf("method = %q", r.Method)
	}
	if r.URI != "/reports/5?p=2" {
		t.Errorf("uri = %q", r.URI)
	}
}

func TestParseAccessRecordRejects(t *testing.T) {
	bad := [][]byte{
		[]byte(""),
		[]byte("<190>Jul  2 10:00:00 lerdaccess: -"),
		[]byte("garbage-with-no-pipes"),
		[]byte("myapp.test|notanumber|0.1|GET|/x"),
	}
	for _, b := range bad {
		if _, ok := ParseAccessRecord(b); ok {
			t.Errorf("expected reject for %q", b)
		}
	}
}

// A plain host-only datagram (old idle format) must not parse as a full record.
func TestParseAccessRecordIgnoresHostOnly(t *testing.T) {
	if _, ok := ParseAccessRecord([]byte("myapp.test")); ok {
		t.Error("host-only line must not parse as a timing record")
	}
}
