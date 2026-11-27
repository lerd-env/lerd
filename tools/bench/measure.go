package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// calibrate returns sha256 throughput in MB/s on one core and on all cores, so
// results from different machines can be read against the machine's own speed.
func calibrate(d time.Duration) (single, multi float64) {
	return hashRate(1, d), hashRate(runtime.NumCPU(), d)
}

func hashRate(workers int, d time.Duration) float64 {
	buf := make([]byte, 1<<20)
	var total atomic.Int64
	var wg sync.WaitGroup
	deadline := time.Now().Add(d)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				sha256.Sum256(buf)
				total.Add(1)
			}
		}()
	}
	wg.Wait()
	return float64(total.Load()) / d.Seconds()
}

// hostRoles maps a lerd process command line to the name it is reported under.
// lerd mcp is left out: it belongs to whichever AI client spawned it.
var hostRoles = []struct{ match, role string }{
	{"lerd serve-ui", "ui"},
	{"lerd watch", "watcher"},
	{"lerd dns-serve", "dns"},
	{"lerd-tray", "tray"},
}

type usage struct {
	cpu   time.Duration
	memMB float64
}

func sampleIdle(window time.Duration, m map[string]float64) error {
	h0, err := hostUsage()
	if err != nil {
		return err
	}
	c0, err := containerUsage()
	if err != nil {
		return err
	}
	t0 := time.Now()
	time.Sleep(window)
	h1, err := hostUsage()
	if err != nil {
		return err
	}
	c1, err := containerUsage()
	if err != nil {
		return err
	}
	wall := time.Since(t0)

	var cpuTotal, rssTotal, memTotal float64
	for role, u := range h1 {
		pct := cpuPct(h0[role].cpu, u.cpu, wall)
		m["idle.cpu_pct.host."+role] = pct
		m["idle.rss_mb.host."+role] = u.memMB
		cpuTotal += pct
		rssTotal += u.memMB
	}
	for name, u := range c1 {
		pct := cpuPct(c0[name].cpu, u.cpu, wall)
		m["idle.cpu_pct.ctr."+name] = pct
		m["idle.mem_mb.ctr."+name] = u.memMB
		cpuTotal += pct
		memTotal += u.memMB
	}
	m["idle.cpu_pct.total"] = cpuTotal
	m["idle.rss_mb.host_total"] = rssTotal
	m["idle.mem_mb.ctr_total"] = memTotal
	m["containers.count"] = float64(len(c1))
	return nil
}

func cpuPct(before, after, wall time.Duration) float64 {
	if after < before {
		return 0 // process restarted during the window
	}
	return float64(after-before) / float64(wall) * 100
}

// hostUsage sums CPU time and RSS of lerd's own host processes per role.
func hostUsage() (map[string]usage, error) {
	out, err := exec.Command("ps", "-axo", "pid=,rss=,time=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	res := map[string]usage{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		role := roleOf(strings.Join(f[3:], " "))
		if role == "" {
			continue
		}
		rssKB, _ := strconv.ParseFloat(f[1], 64)
		cpu, err := procCPU(f[0], f[2])
		if err != nil {
			return nil, err
		}
		u := res[role]
		u.cpu += cpu
		u.memMB += rssKB / 1024
		res[role] = u
	}
	return res, nil
}

func roleOf(args string) string {
	exe, rest, _ := strings.Cut(args, " ")
	cmd := strings.TrimSpace(filepath.Base(exe) + " " + rest)
	for _, r := range hostRoles {
		if cmd == r.match || strings.HasPrefix(cmd, r.match+" ") {
			return r.role
		}
	}
	return ""
}

// procCPU reads a process's CPU time. On Linux it comes from /proc, since ps
// rounds to whole seconds, and includes reaped subprocesses such as podman,
// which is most of what polling costs; macOS ps has no children figure.
func procCPU(pid, psTime string) (time.Duration, error) {
	if runtime.GOOS != "linux" {
		return parsePSTime(psTime)
	}
	b, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return 0, nil // exited between ps and now
	}
	return parseProcStat(string(b))
}

// parseProcStat returns utime+stime+cutime+cstime from /proc/<pid>/stat,
// assuming USER_HZ=100.
func parseProcStat(s string) (time.Duration, error) {
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, fmt.Errorf("unexpected /proc stat: %q", s)
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 15 {
		return 0, fmt.Errorf("unexpected /proc stat: %q", s)
	}
	var ticks int64
	for _, field := range f[11:15] {
		n, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("unexpected /proc stat: %q", s)
		}
		ticks += n
	}
	return time.Duration(ticks) * 10 * time.Millisecond, nil
}

// parsePSTime parses ps's cputime, [DD-][HH:]MM:SS[.ss] on both Linux and macOS.
func parsePSTime(s string) (time.Duration, error) {
	var days int64
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.ParseInt(d, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bad ps time %q", s)
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("bad ps time %q", s)
	}
	secs, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return 0, fmt.Errorf("bad ps time %q", s)
	}
	total := secs + float64(days)*86400
	for i, mult := range []float64{60, 3600} {
		idx := len(parts) - 2 - i
		if idx < 0 {
			break
		}
		n, err := strconv.ParseInt(parts[idx], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bad ps time %q", s)
		}
		total += float64(n) * mult
	}
	return time.Duration(total * float64(time.Second)), nil
}

func containerUsage() (map[string]usage, error) {
	out, err := podman.Run("stats", "--no-stream", "--format", "json")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Name    string `json:"name"`
		CPUTime string `json:"cpu_time"`
		Mem     string `json:"mem_usage"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, fmt.Errorf("podman stats: %w", err)
	}
	res := map[string]usage{}
	for _, r := range rows {
		if !strings.HasPrefix(r.Name, "lerd-") {
			continue
		}
		cpu, err := time.ParseDuration(r.CPUTime)
		if err != nil {
			return nil, fmt.Errorf("podman stats %s cpu_time: %w", r.Name, err)
		}
		used, _, _ := strings.Cut(r.Mem, " / ")
		mb, err := parseSizeMB(used)
		if err != nil {
			return nil, fmt.Errorf("podman stats %s: %w", r.Name, err)
		}
		res[r.Name] = usage{cpu: cpu, memMB: mb}
	}
	return res, nil
}

// parseSizeMB reads podman's human sizes ("193.8MB", "1.2GiB", "512kB") as MB.
func parseSizeMB(s string) (float64, error) {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		mb     float64
	}{
		{"KiB", 1024.0 / 1e6}, {"MiB", 1048576.0 / 1e6}, {"GiB", 1073741824.0 / 1e6},
		{"kB", 1e-3}, {"MB", 1}, {"GB", 1e3}, {"TB", 1e6}, {"B", 1e-6},
	}
	for _, u := range units {
		if num, ok := strings.CutSuffix(s, u.suffix); ok {
			n, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err != nil {
				return 0, fmt.Errorf("bad size %q", s)
			}
			return n * u.mb, nil
		}
	}
	return 0, fmt.Errorf("bad size %q", s)
}

const fixtureName = "bench-fixture"

// linkFixture links a one-line PHP site and returns its URL and an undo that
// unlinks it again, unless it was already linked before the run.
func linkFixture() (string, func(), error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Lerd", fixtureName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php echo 'ok';\n"), 0o644); err != nil {
		return "", nil, err
	}
	url := "http://" + fixtureName + ".test/"
	undo := func() {}
	sites, _ := runCmd("lerd", "sites")
	if !strings.Contains(sites, fixtureName+".test") {
		link := exec.Command("lerd", "link", "--yes")
		link.Dir = dir
		if out, err := link.CombinedOutput(); err != nil {
			return "", nil, fmt.Errorf("lerd link: %v: %s", err, strings.TrimSpace(string(out)))
		}
		undo = func() {
			unlink := exec.Command("lerd", "unlink")
			unlink.Dir = dir
			if out, err := unlink.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "bench: lerd unlink: %v: %s\n", err, out)
			}
		}
	}
	if err := waitOK(newClient(), url, 30*time.Second); err != nil {
		undo()
		return "", nil, err
	}
	return url, undo, nil
}

func newClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		MaxIdleConnsPerHost: 16,
	}}
}

// measureHTTP records sequential latency and throughput under 16 clients.
func measureHTTP(url string, m map[string]float64) error {
	client := newClient()
	var lat []float64
	for i := 0; i < 55; i++ {
		t := time.Now()
		if err := get(client, url); err != nil {
			return err
		}
		if i >= 5 { // the first few warm FPM and the connection
			lat = append(lat, float64(time.Since(t).Microseconds())/1000)
		}
	}
	m["http.ttfb_ms_median"] = median(lat)

	rps, p95, err := load(client, url, 16, 10*time.Second)
	if err != nil {
		return err
	}
	m["http.rps_c16"] = rps
	m["http.p95_ms_c16"] = p95
	return nil
}

func get(c *http.Client, url string) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return nil
}

func waitOK(c *http.Client, url string, within time.Duration) error {
	deadline := time.Now().Add(within)
	for {
		err := get(c, url)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fixture never answered: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// load runs n keep-alive clients for d and returns requests/s and p95 latency.
func load(c *http.Client, url string, n int, d time.Duration) (rps, p95 float64, err error) {
	var mu sync.Mutex
	var lat []float64
	var firstErr error
	var wg sync.WaitGroup
	deadline := time.Now().Add(d)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local []float64
			for time.Now().Before(deadline) {
				t := time.Now()
				if e := get(c, url); e != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = e
					}
					mu.Unlock()
					return
				}
				local = append(local, float64(time.Since(t).Microseconds())/1000)
			}
			mu.Lock()
			lat = append(lat, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return 0, 0, firstErr
	}
	return float64(len(lat)) / d.Seconds(), percentile(lat, 0.95), nil
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[int(float64(len(s)-1)*p)]
}

func cpuModel() string {
	if runtime.GOOS == "darwin" {
		out, _ := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		return strings.TrimSpace(string(out))
	}
	b, _ := os.ReadFile("/proc/cpuinfo")
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "model name" {
			return strings.TrimSpace(v)
		}
	}
	return "unknown"
}

// loadAvg returns the one-minute load average, or -1 when it cannot be read.
func loadAvg() float64 {
	var raw string
	if runtime.GOOS == "darwin" {
		out, _ := exec.Command("sysctl", "-n", "vm.loadavg").Output() // "{ 1.23 1.10 0.98 }"
		raw = strings.Trim(strings.TrimSpace(string(out)), "{ }")
	} else {
		b, _ := os.ReadFile("/proc/loadavg")
		raw = string(b)
	}
	f := strings.Fields(raw)
	if len(f) == 0 {
		return -1
	}
	n, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return -1
	}
	return n
}

func ramMB() int {
	if runtime.GOOS == "darwin" {
		out, _ := exec.Command("sysctl", "-n", "hw.memsize").Output()
		n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return int(n / 1048576)
	}
	b, _ := os.ReadFile("/proc/meminfo")
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && f[0] == "MemTotal:" {
			kb, _ := strconv.ParseInt(f[1], 10, 64)
			return int(kb / 1024)
		}
	}
	return 0
}
