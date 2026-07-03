// Package spxreport reads the SPX profiler's on-disk captures and distills each
// into a handful of timing outliers. SPX writes one report per profiled request
// as a .json metadata sidecar (method, host, uri, timings) plus a gzipped .txt
// event trace. This package turns the freshest capture of a route into the top
// functions by exclusive wall time, so the join in optimize_route can hand an
// agent the CPU hotspots behind a slow route rather than a 400k-line trace.
package spxreport

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

// Hotspot is one function's share of a request's wall time. Exclusive time (self,
// not descendants) is what points at the code to change.
type Hotspot struct {
	Function string  `json:"function"`
	ExclMS   float64 `json:"excl_ms"`
	Calls    int     `json:"calls"`
	Pct      float64 `json:"pct"`
}

// Profile is the distilled view of one capture: when it was taken, the request it
// came from, its total wall time, and only the top timing outliers.
type Profile struct {
	CapturedAt time.Time `json:"captured_at"`
	Method     string    `json:"method"`
	URI        string    `json:"uri"`
	WallMS     float64   `json:"wall_ms"`
	Hotspots   []Hotspot `json:"hotspots"`
}

// reportMeta mirrors the fields read from a capture's .json sidecar.
type reportMeta struct {
	ExecTS     int64  `json:"exec_ts"`
	HTTPMethod string `json:"http_method"`
	HTTPHost   string `json:"http_host"`
	HTTPURI    string `json:"http_request_uri"`
	CLI        int    `json:"cli"`
}

// ProfilesForRoutes scans dataDir once and, for each of the wanted normalized
// routes served on one of hosts, parses the freshest matching capture into its
// top hotspots. Routes with no capture (profiler was off, or the route was not
// hit while profiling) are simply absent from the returned map.
func ProfilesForRoutes(dataDir string, hosts, routes []string, topN int, minPct float64) map[string]Profile {
	want := make(map[string]bool, len(routes))
	for _, r := range routes {
		want[r] = true
	}
	hostSet := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		hostSet[h] = true
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil
	}
	// Freshest capture key + metadata per route.
	type ref struct {
		key  string
		meta reportMeta
	}
	best := map[string]ref{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil {
			continue
		}
		var m reportMeta
		if json.Unmarshal(b, &m) != nil || m.CLI != 0 || !hostSet[m.HTTPHost] {
			continue
		}
		route := reqstats.NormalizeRoute(m.HTTPMethod, m.HTTPURI)
		if !want[route] {
			continue
		}
		if cur, ok := best[route]; !ok || m.ExecTS > cur.meta.ExecTS {
			best[route] = ref{key: strings.TrimSuffix(name, ".json"), meta: m}
		}
	}

	out := make(map[string]Profile, len(best))
	for route, r := range best {
		prof, err := parseTrace(filepath.Join(dataDir, r.key+".txt.gz"), topN, minPct)
		if err != nil {
			continue
		}
		prof.CapturedAt = time.Unix(r.meta.ExecTS, 0).UTC()
		prof.Method = r.meta.HTTPMethod
		prof.URI = r.meta.HTTPURI
		out[route] = prof
	}
	return out
}

func parseTrace(path string, topN int, minPct float64) (Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return Profile{}, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Profile{}, err
	}
	defer gz.Close()
	return parseFlat(gz, topN, minPct)
}

// nsPerMS converts the trace's cumulative wall time (nanoseconds) to milliseconds.
const nsPerMS = 1_000_000.0

// parseFlat walks an SPX text trace into a flat profile, keeping only the top
// hotspots by exclusive wall time. The trace is an [events] section of
// "idx dir wall mem" lines (wall is cumulative nanoseconds, dir 1=enter 0=exit)
// followed by a [functions] name table indexed by line order. Exclusive time for
// the function on top of the call stack accrues over the gap between two events.
func parseFlat(r io.Reader, topN int, minPct float64) (Profile, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	type event struct {
		idx   int
		enter bool
		wall  int64
	}
	var events []event
	var funcs []string
	section := ""
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if line[0] == '[' {
			section = line
			continue
		}
		switch section {
		case "[events]":
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			idx, _ := strconv.Atoi(fields[0])
			wall, _ := strconv.ParseInt(fields[2], 10, 64)
			events = append(events, event{idx: idx, enter: fields[1] == "1", wall: wall})
		case "[functions]":
			funcs = append(funcs, line)
		}
	}
	if err := sc.Err(); err != nil {
		return Profile{}, err
	}

	excl := make([]int64, len(funcs))
	calls := make([]int, len(funcs))
	var stack []int
	var prev int64
	for _, e := range events {
		if len(stack) > 0 {
			if top := stack[len(stack)-1]; top < len(excl) {
				excl[top] += e.wall - prev
			}
		}
		prev = e.wall
		if e.enter {
			if e.idx < len(calls) {
				calls[e.idx]++
			}
			stack = append(stack, e.idx)
		} else if len(stack) > 0 {
			stack = stack[:len(stack)-1]
		}
	}

	var total int64
	for _, v := range excl {
		total += v
	}
	order := make([]int, 0, len(funcs))
	for i, v := range excl {
		if v > 0 {
			order = append(order, i)
		}
	}
	sort.Slice(order, func(i, j int) bool { return excl[order[i]] > excl[order[j]] })

	prof := Profile{WallMS: round1(float64(total) / nsPerMS), Hotspots: []Hotspot{}}
	for _, i := range order {
		pct := 0.0
		if total > 0 {
			pct = float64(excl[i]) / float64(total) * 100
		}
		// Keep the single biggest even if it's below the floor, so a route always
		// shows its worst offender; stop once we hit the cap or the trivial tail.
		if len(prof.Hotspots) >= topN || (len(prof.Hotspots) > 0 && pct < minPct) {
			break
		}
		prof.Hotspots = append(prof.Hotspots, Hotspot{
			Function: funcs[i],
			ExclMS:   round1(float64(excl[i]) / nsPerMS),
			Calls:    calls[i],
			Pct:      round1(pct),
		})
	}
	return prof, nil
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }
