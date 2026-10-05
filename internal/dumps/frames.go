package dumps

import "encoding/json"

// Stack traces are most of what the ring holds: every query, cache call and
// log line carries one, and they repeat the same framework frames over and
// over. The ring keeps each distinct frame once and an event only the numbers
// of its frames, building the trace again for the events someone reads.

// maxFrames caps the table; past it a trace is kept as it came, so a long-lived
// lerd-ui on a machine full of projects never grows the table without bound.
const maxFrames = 200000

type frameTable struct {
	ids  map[string]uint32
	list []json.RawMessage
	// keyed holds the traces a request sent once under a key, by request id
	// and key, for the later events that repeat only the key.
	keyed map[string][]uint32
}

// maxKeyed caps the keyed traces kept; past it a repeat goes without one.
const maxKeyed = 200000

func newFrameTable() *frameTable {
	return &frameTable{ids: map[string]uint32{}, keyed: map[string][]uint32{}}
}

// strip takes the trace out of e's data and returns the event without it and
// the trace as frame numbers, or e untouched and nil when it has no trace or
// the table is full.
func (t *frameTable) strip(e Event) (Event, []uint32) {
	if len(e.Data) == 0 || e.Data[0] != '{' {
		return e, nil
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(e.Data, &data) != nil {
		return e, nil
	}
	var key string
	if k, ok := data["trace_key"]; ok {
		_ = json.Unmarshal(k, &key)
		delete(data, "trace_key")
		key = e.Ctx.RID + "|" + key
	}
	raw, ok := data["trace"]
	if !ok {
		// A repeat of a trace its request sent before: the key says which.
		ids, known := t.keyed[key]
		if key == "" || !known {
			return e, nil
		}
		if stripped, err := json.Marshal(data); err == nil {
			e.Data = stripped
		}
		return e, ids
	}
	var frames []json.RawMessage
	if json.Unmarshal(raw, &frames) != nil || len(t.list)+len(frames) > maxFrames {
		return e, nil
	}
	ids := make([]uint32, 0, len(frames))
	for _, f := range frames {
		key := string(f)
		id, ok := t.ids[key]
		if !ok {
			id = uint32(len(t.list))
			t.ids[key] = id
			t.list = append(t.list, f)
		}
		ids = append(ids, id)
	}
	delete(data, "trace")
	stripped, err := json.Marshal(data)
	if err != nil {
		return e, nil
	}
	if key != "" && len(t.keyed) < maxKeyed {
		t.keyed[key] = ids
	}
	e.Data = stripped
	return e, ids
}

// frames is the trace ids name, as JSON.
func (t *frameTable) frames(ids []uint32) json.RawMessage {
	list := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		if int(id) < len(t.list) {
			list = append(list, t.list[id])
		}
	}
	out, _ := json.Marshal(list)
	return out
}

// restore puts the trace given by ids back into e's data.
func (t *frameTable) restore(e Event, ids []uint32) Event {
	if ids == nil {
		return e
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(e.Data, &data) != nil {
		return e
	}
	frames := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		if int(id) < len(t.list) {
			frames = append(frames, t.list[id])
		}
	}
	trace, _ := json.Marshal(frames)
	data["trace"] = trace
	if full, err := json.Marshal(data); err == nil {
		e.Data = full
	}
	return e
}
