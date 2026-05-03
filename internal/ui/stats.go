package ui

import (
	"context"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// ContainerStat is one row in the resources panel: a single lerd-prefixed
// container with the cheap metrics podman exposes (no per-process digging).
type ContainerStat struct {
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemBytes   int64   `json:"mem_bytes"`
	MemLimit   int64   `json:"mem_limit_bytes"`
	MemPercent float64 `json:"mem_percent"`
}

// StatsResponse is the JSON the dashboard polls. Totals are computed
// server-side so the client doesn't have to re-aggregate.
type StatsResponse struct {
	Containers      []ContainerStat `json:"containers"`
	TotalCPUPercent float64         `json:"total_cpu_percent"`
	TotalMemBytes   int64           `json:"total_mem_bytes"`
	HostMemBytes    int64           `json:"host_mem_bytes"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Available       bool            `json:"available"`
}

const (
	statsCacheTTL = 3 * time.Second
	statsTimeout  = 4 * time.Second
)

// statsFn is swappable for tests so the handler can be exercised without
// shelling out to a real podman binary.
var statsFn = readPodmanStats

var (
	statsMu       sync.Mutex
	statsCached   *StatsResponse
	statsCachedAt time.Time
)

// handleStats returns the latest container stats, cached briefly to absorb
// many open tabs polling at once. podman stats with --no-stream takes
// ~100-300ms; cache TTL of 3s keeps the host load negligible even with a
// dashboard, the System tab, and a TUI client all watching.
func handleStats(w http.ResponseWriter, _ *http.Request) {
	statsMu.Lock()
	if statsCached != nil && time.Since(statsCachedAt) < statsCacheTTL {
		cached := *statsCached
		statsMu.Unlock()
		writeJSON(w, cached)
		return
	}
	statsMu.Unlock()

	resp := buildStatsResponse()

	statsMu.Lock()
	statsCached = &resp
	statsCachedAt = time.Now()
	statsMu.Unlock()

	writeJSON(w, resp)
}

func buildStatsResponse() StatsResponse {
	out := StatsResponse{
		Containers: []ContainerStat{},
		UpdatedAt:  time.Now(),
	}

	rows, err := statsFn()
	if err != nil || len(rows) == 0 {
		return out
	}

	out.Available = true
	out.Containers = rows

	for _, r := range rows {
		out.TotalCPUPercent += r.CPUPercent
		out.TotalMemBytes += r.MemBytes
		if r.MemLimit > out.HostMemBytes {
			out.HostMemBytes = r.MemLimit
		}
	}

	// Sort by memory desc so the dashboard's top-consumers list is ordered.
	sort.Slice(out.Containers, func(i, j int) bool {
		return out.Containers[i].MemBytes > out.Containers[j].MemBytes
	})

	return out
}

// readPodmanStats invokes `podman stats --no-stream` with a pipe-delimited
// template and parses the rows. Filters to containers prefixed `lerd-` so
// we never accidentally surface the user's other unrelated containers.
func readPodmanStats() ([]ContainerStat, error) {
	// Bound the wall-clock cost so a wedged podman never stalls a request.
	ctx, cancel := context.WithTimeout(context.Background(), statsTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		podman.PodmanBin(), "stats", "--no-stream",
		"--format", "{{.Name}}|{{.CPU}}|{{.MemUsage}}|{{.MemPerc}}",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseStatsRows(string(out)), nil
}

func parseStatsRows(text string) []ContainerStat {
	var rows []ContainerStat
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(name, "lerd-") {
			continue
		}
		cpu, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		used, limit := parseMemUsage(parts[2])
		memPerc, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[3]), "%"), 64)
		rows = append(rows, ContainerStat{
			Name:       name,
			CPUPercent: cpu,
			MemBytes:   used,
			MemLimit:   limit,
			MemPercent: memPerc,
		})
	}
	return rows
}

// parseMemUsage takes podman's "139.5MB / 33.23GB" string and returns the
// two values in bytes. Robust to extra whitespace; returns 0 on parse
// failure rather than erroring (a single bad row shouldn't drop the rest).
func parseMemUsage(s string) (used, limit int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseSize(parts[0]), parseSize(parts[1])
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Pull the trailing alpha tail off as the unit; the rest is the number.
	splitAt := len(s)
	for i, r := range s {
		if (r < '0' || r > '9') && r != '.' && r != '-' && r != '+' && r != 'e' && r != 'E' {
			splitAt = i
			break
		}
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(s[:splitAt]), 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(strings.TrimSpace(s[splitAt:]))
	mult := float64(1)
	switch unit {
	case "", "b":
		mult = 1
	case "k", "kb", "kib":
		mult = 1024
	case "m", "mb", "mib":
		mult = 1024 * 1024
	case "g", "gb", "gib":
		mult = 1024 * 1024 * 1024
	case "t", "tb", "tib":
		mult = 1024 * 1024 * 1024 * 1024
	}
	return int64(num * mult)
}
