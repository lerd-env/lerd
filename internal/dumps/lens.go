package dumps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

// What the Debug lenses group, search and narrow on, worked out once when an
// event is stored so every lens is a query rather than a pass over events in
// a browser tab.

// groupKey buckets an event into one request: its id when the extension gave
// it one, else method+path+pid for a web request, else a 5s pid bucket for a
// CLI run. Site and branch lead both fallbacks so a worktree's request never
// merges into its parent's. dump() can run without the extension, whose id then
// changes per call, so dumps always group by request.
func groupKey(e Event) string {
	if e.Ctx.RID != "" && e.Kind != KindDump {
		return "rid:" + e.Ctx.RID
	}
	if e.Ctx.Type == "fpm" {
		return fmt.Sprintf("fpm:%s:%s:%s:%d", e.Ctx.Site, e.Ctx.Branch, e.Ctx.Request, e.Ctx.PID)
	}
	var bucket int64
	if t, err := time.Parse(time.RFC3339Nano, e.TS); err == nil {
		bucket = t.UnixMilli() / 5000
	}
	return fmt.Sprintf("cli:%s:%s:%d:%d", e.Ctx.Site, e.Ctx.Branch, e.Ctx.PID, bucket)
}

var requestLine = regexp.MustCompile(`^([A-Z]+) (/\S*)$`)

// routeOf is the route an event ran under, as the timing view names it: a web
// request's "METHOD /uri", a browser event's page as a GET, or "" for a CLI run.
func routeOf(e Event) string {
	if m := requestLine.FindStringSubmatch(e.Ctx.Request); m != nil {
		return reqstats.NormalizeRoute(m[1], m[2])
	}
	if e.Ctx.Type != "browser" {
		return ""
	}
	u, err := url.Parse(e.Ctx.Request)
	if err != nil || u.Scheme == "" {
		return ""
	}
	return reqstats.NormalizeRoute("GET", u.RequestURI())
}

// haystack is the lowercased text a lens search matches: a dump's label and
// output, a query's SQL, any other kind's whole payload, plus the request, the
// worker, the branch, the request id and the route for all of them.
func haystack(e Event) string {
	var parts []string
	switch e.Kind {
	case KindDump:
		parts = []string{e.Label, e.Text, e.Ctx.Request, e.Src.File}
	case KindQuery:
		q, _ := e.Query()
		parts = []string{q.SQL, e.Ctx.Request, e.Src.File, e.Ctx.Worker}
	default:
		data := e.Data
		var compact bytes.Buffer
		if json.Compact(&compact, data) == nil {
			data = compact.Bytes()
		}
		parts = []string{string(data), e.Ctx.Request, e.Ctx.Worker}
	}
	parts = append(parts, e.Ctx.Branch, e.Ctx.RID, routeOf(e))
	return strings.ToLower(strings.Join(parts, " "))
}

// facetOf is the one value a kind narrows on: a job's status, a log's level,
// a message's channel, a browser event's type with console messages split by
// level.
func facetOf(e Event) string {
	var d struct {
		Status  string `json:"status"`
		Level   string `json:"level"`
		Type    string `json:"type"`
		Channel string `json:"channel"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return ""
	}
	if e.Kind == KindMessage {
		return d.Channel
	}
	if e.Kind == KindBrowser {
		if d.Type == "console" {
			return "console." + d.Level
		}
		return d.Type
	}
	if d.Status != "" {
		return d.Status
	}
	return d.Level
}

// jobUUID is a job event's uuid, the key its rows share across dispatch and
// the worker, and whether the event carries the job's payload.
func jobUUID(e Event) (uuid string, hasPayload bool) {
	if e.Kind != KindJob {
		return "", false
	}
	var d struct {
		UUID    string          `json:"uuid"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return "", false
	}
	return d.UUID, len(d.Payload) > 0 && string(d.Payload) != "null"
}

// withPayload returns e's data with payload set, keeping every other field.
func withPayload(data, payload json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil {
		return data
	}
	m["payload"] = payload
	out, err := json.Marshal(m)
	if err != nil {
		return data
	}
	return out
}

// detailFields are what only an expanded row shows: a call stack and a mail's
// rendered HTML. A lens lists rows without them and reads the event to open one.
var detailFields = []string{"trace", "html"}

// brief is the event as a lens lists it, data being the whole event encoded.
func brief(e Event, data []byte) []byte {
	var d map[string]json.RawMessage
	if json.Unmarshal(e.Data, &d) != nil {
		return data
	}
	cut := false
	for _, f := range detailFields {
		if _, ok := d[f]; ok {
			delete(d, f)
			cut = true
		}
	}
	if !cut {
		return data
	}
	short := e
	short.Data, _ = json.Marshal(d)
	out, err := json.Marshal(short)
	if err != nil {
		return data
	}
	return out
}

var (
	sqlSingleQuoted = regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)
	sqlDoubleQuoted = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	sqlNumber       = regexp.MustCompile(`\b\d+\b`)
	sqlSpace        = regexp.MustCompile(`\s+`)
)

// normalizeSQL collapses literals so structurally identical queries share a
// fingerprint, which is what the duplicate and N+1 flags count.
func normalizeSQL(sql string) string {
	sql = sqlSingleQuoted.ReplaceAllString(sql, "?")
	sql = sqlDoubleQuoted.ReplaceAllString(sql, "?")
	sql = sqlNumber.ReplaceAllString(sql, "?")
	sql = sqlSpace.ReplaceAllString(sql, " ")
	return strings.ToLower(strings.TrimSpace(sql))
}
