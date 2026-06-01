// Package dumps receives, buffers, and fans out PHP `dump()`/`dd()` events
// captured by the lerd debug bridge (auto_prepend_file). The wire format is
// newline-delimited JSON. Production lerd-ui listens on a per-user Unix
// socket; tests bind TCP loopback. See docs/features/dumps.md.
package dumps

import "encoding/json"

// ProtocolVersion is the wire-format version this package understands.
// Events with a different `v` are dropped.
const ProtocolVersion = 1

// Event kinds. KindDump (dump()/dd() output) was the first; the rest are
// emitted by the lerd_devtools Zend extension and the per-framework adapters
// (Laravel, Symfony). New kinds are added without bumping ProtocolVersion —
// the buffer and fan-out treat every kind identically, only per-kind
// consumers (UI lenses, query analysis) interpret Data.
const (
	KindDump      = "dump"
	KindQuery     = "query"
	KindJob       = "job"
	KindView      = "view"
	KindMail      = "mail"
	KindCache     = "cache"
	KindEvent     = "event"
	KindHTTP      = "http"
	KindLog       = "log"
	KindException = "exception"
)

// Source identifies the file:line that produced a dump.
type Source struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Context describes where a dump came from. Type is "fpm" (web request) or
// "cli" (artisan, tinker, queue worker). Empty fields are omitted on the wire.
// Branch is non-empty only when the event originated inside a git worktree
// (set by the bridge from LERD_BRANCH, injected by nginx for worktree vhosts
// and by lerd's CLI helpers when shelling into a worktree path).
type Context struct {
	Type    string `json:"type"`
	Site    string `json:"site,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Domain  string `json:"domain,omitempty"`
	Request string `json:"request,omitempty"`
	PID     int    `json:"pid,omitempty"`
	// RID is a unique per-request (FPM) / per-invocation (CLI) id stamped by
	// the lerd_devtools extension so consumers can group events by the exact
	// request, not just method+path+pid (which collapses repeat hits to the
	// same URL on a reused pool worker). Empty for dump-bridge events.
	RID string `json:"rid,omitempty"`
	// Worker names the queue/scheduler command this event came from (e.g.
	// "queue:work", "scrape:rtb-data"). Set only for worker-process events,
	// which are captured solely when the user opts in.
	Worker string `json:"worker,omitempty"`
}

// Event is one captured payload. For dumps, Text holds the rendered
// VarDumper output and Tree the pre-walked cloner tree. For every other
// kind, Data carries the kind-specific structured fields as opaque JSON
// (a query's sql/bindings/time, a job's class/status, …). The ring and hub
// never decode Data; only the UI lens and analysis helpers do.
type Event struct {
	V     int             `json:"v"`
	ID    string          `json:"id"`
	TS    string          `json:"ts"`
	Kind  string          `json:"kind"`
	Ctx   Context         `json:"ctx"`
	Src   Source          `json:"src"`
	Label string          `json:"label,omitempty"`
	Text  string          `json:"text,omitempty"`
	Tree  json.RawMessage `json:"tree,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	Trunc bool            `json:"trunc,omitempty"`
}

// QueryData is the Data payload for KindQuery events. The lerd_devtools
// extension fills sql/bindings/time_ms from the agnostic PDO/mysqli hook;
// the Laravel adapter (QueryExecuted) additionally sets Connection and
// RWType. The originating file:line lives in Event.Src, like dumps.
type QueryData struct {
	SQL        string        `json:"sql"`
	Bindings   []interface{} `json:"bindings,omitempty"`
	TimeMS     float64       `json:"time_ms"`
	Connection string        `json:"connection,omitempty"`
	RWType     string        `json:"rw_type,omitempty"`
}

// Query decodes the Data payload as a QueryData. ok is false when the event
// is not a query or its payload does not decode.
func (e Event) Query() (QueryData, bool) {
	if e.Kind != KindQuery || len(e.Data) == 0 {
		return QueryData{}, false
	}
	var q QueryData
	if err := json.Unmarshal(e.Data, &q); err != nil {
		return QueryData{}, false
	}
	return q, true
}

// Valid reports whether an event passes the minimum schema check applied by
// the listener before it is appended to the ring.
func (e Event) Valid() bool {
	return e.V == ProtocolVersion && e.ID != "" && e.Kind != ""
}
