// Command bench measures what the installed lerd costs on this machine (RAM,
// CPU, start time, request latency) and appends the result to a local history
// so versions, hosts and VMs can be compared. Run it with `go run ./tools/bench`.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Record is one bench run, one line of history.jsonl.
type Record struct {
	Time       time.Time          `json:"time"`
	Host       string             `json:"host"`
	OS         string             `json:"os"`
	Arch       string             `json:"arch"`
	CPU        string             `json:"cpu"`
	Cores      int                `json:"cores"`
	RAMMB      int                `json:"ram_mb"`
	Lerd       string             `json:"lerd"`
	Sites      int                `json:"sites"`
	Containers int                `json:"containers"`
	Runs       int                `json:"runs,omitempty"`
	Load1      float64            `json:"load1"`
	Metrics    map[string]float64 `json:"metrics"`
	// Noise is each metric's spread across the runs, (max-min)/median.
	Noise map[string]float64 `json:"noise,omitempty"`
}

func main() {
	cold := flag.Bool("cold", false, "stop and start lerd to time a cold start (restarts your stack)")
	idle := flag.Duration("idle", 10*time.Second, "window for sampling idle CPU")
	squeezeCPU := flag.Bool("squeeze", false, "throttle the fixture's nginx and FPM to find the minimum CPUs")
	flag.BoolVar(&squeezeShared, "squeeze-shared", false, "squeeze even when other sites share the fixture's PHP container (they are throttled too)")
	runs := flag.Int("runs", 3, "repetitions; each metric is the median across them")
	history := flag.String("history", defaultHistory(), "history file to append to")
	flag.Parse()

	if err := run(*cold, *squeezeCPU, *idle, *runs, *history); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func run(cold, squeezeCPU bool, idle time.Duration, runs int, history string) error {
	// Links and CPU limits register their undo as they happen; it runs once,
	// on return, on error and on Ctrl+C alike.
	defer cleanup.run()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		if _, ok := <-sig; ok {
			cleanup.run()
			os.Exit(130)
		}
	}()
	if runs < 1 {
		return fmt.Errorf("-runs must be at least 1, got %d", runs)
	}
	host, _ := os.Hostname()
	rec := Record{Time: time.Now().UTC(), Host: host, OS: runtime.GOOS, Arch: runtime.GOARCH,
		CPU: cpuModel(), Cores: runtime.NumCPU(), RAMMB: ramMB(), Lerd: lerdVersion(), Runs: runs}
	samples := map[string][]float64{}
	rec.Load1 = loadAvg()
	if rec.Load1 > float64(rec.Cores)*0.75 {
		fmt.Fprintf(os.Stderr, "WARNING: load %.1f on %d cores, other work will skew the numbers\n", rec.Load1, rec.Cores)
	}

	if cold {
		step("timing a cold lerd start")
		if _, err := runCmd("lerd", "stop"); err != nil {
			return fmt.Errorf("lerd stop: %w", err)
		}
		d, err := timeCmd("lerd", "start")
		if err != nil {
			return fmt.Errorf("lerd start: %w", err)
		}
		samples["start.cold_s"] = []float64{d.Seconds()}
		// Let workers and containers finish booting so idle numbers are idle.
		time.Sleep(20 * time.Second)
	}

	step("linking the fixture site")
	url, err := linkFixture()
	if err != nil {
		return err
	}

	for i := 1; i <= runs; i++ {
		step(fmt.Sprintf("run %d/%d", i, runs))
		m := map[string]float64{}
		m["calib.single_mbps"], m["calib.multi_mbps"] = calibrate(2 * time.Second)
		if err := sampleIdle(idle, m); err != nil {
			return err
		}
		m["cli.sites_ms"] = medianCmd(5, "lerd", "sites")
		if err := measureHTTP(url, m); err != nil {
			return err
		}
		for k, v := range m {
			samples[k] = append(samples[k], v)
		}
		time.Sleep(3 * time.Second) // let FPM children settle before the next idle window
	}
	rec.Metrics, rec.Noise = summarise(samples)
	if squeezeCPU {
		step("squeezing CPU on a framework page")
		if err := squeeze(rec.Metrics); err != nil {
			return err
		}
	}
	sites, err := runCmd("lerd", "sites")
	if err != nil {
		return err
	}
	rec.Sites = userSiteCount(sites)
	rec.Containers = int(rec.Metrics["containers.count"])

	prev, err := lastRecord(history, rec.Host)
	if err != nil {
		return err
	}
	if err := appendRecord(history, rec); err != nil {
		return err
	}
	findings := printReport(rec, prev)
	printRequirements(rec)
	fmt.Println("history:", history)
	if len(findings) > 0 {
		return fmt.Errorf("%d regression(s): %s", len(findings), strings.Join(findings, ", "))
	}
	return nil
}

// summarise reduces repeated samples to a median and a relative spread.
func summarise(samples map[string][]float64) (med, noise map[string]float64) {
	med, noise = map[string]float64{}, map[string]float64{}
	for k, xs := range samples {
		md := median(xs)
		med[k] = md
		lo, hi := xs[0], xs[0]
		for _, x := range xs {
			lo, hi = min(lo, x), max(hi, x)
		}
		if md != 0 {
			noise[k] = (hi - lo) / md
		}
	}
	return med, noise
}

func step(s string) { fmt.Fprintln(os.Stderr, "==>", s) }

func defaultHistory() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "lerd-bench", "history.jsonl")
}

// lastRecord returns the newest run from the same host, or nil when there is none.
func lastRecord(path, host string) (*Record, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var last *Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if r.Host == host {
			last = &r
		}
	}
	return last, sc.Err()
}

func appendRecord(path string, r Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, _ := json.Marshal(r)
	_, err = f.Write(append(line, '\n'))
	return err
}

// printReport prints every metric against the previous run and returns the
// names of those that regressed beyond the noise.
func printReport(cur Record, prev *Record) []string {
	fmt.Printf("\n%s  %s/%s  %s  %d cores  %d MB  sites=%d containers=%d  runs=%d\nlerd %s\nCPU %% is of one core.\n\n",
		cur.Host, cur.OS, cur.Arch, cur.CPU, cur.Cores, cur.RAMMB, cur.Sites, cur.Containers, cur.Runs, cur.Lerd)
	if prev != nil {
		fmt.Printf("compared with %s (lerd %s, sites=%d containers=%d)\n",
			prev.Time.Local().Format("2006-01-02 15:04"), prev.Lerd, prev.Sites, prev.Containers)
		if cur.Runs < 3 || prev.Runs < 3 {
			fmt.Println("no verdicts: both runs need -runs 3 or more to know the noise")
			prev.Metrics = nil
		}
		if prev.Sites != cur.Sites || prev.Containers != cur.Containers {
			fmt.Println("WARNING: different sites or containers running, totals are not comparable")
		}
		fmt.Println()
	}
	keys := make([]string, 0, len(cur.Metrics))
	for k := range cur.Metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var regressed []string
	for _, k := range keys {
		line := fmt.Sprintf("%-38s %12.2f  ±%3.0f%%", k, cur.Metrics[k], cur.Noise[k]*100)
		if prev != nil {
			if p, ok := prev.Metrics[k]; ok {
				v := verdict(k, p, cur.Metrics[k], max(prev.Noise[k], cur.Noise[k]))
				line += fmt.Sprintf("  %-8s %s", delta(p, cur.Metrics[k]), v)
				if v == "REGRESSED" {
					regressed = append(regressed, k)
				}
			}
		}
		fmt.Println(line)
	}
	return regressed
}

// printRequirements prints the game-style spec when the squeeze ran.
func printRequirements(r Record) {
	m := r.Metrics
	if _, ok := m["req.min_cpus"]; !ok {
		return
	}
	fmt.Printf("\nRequirements for one framework site, %s, measured on %s (load %.1f):\n", r.OS, r.CPU, r.Load1)
	if m["req.min_cpus"] < 0 {
		fmt.Printf("  fails even at %g cores, rerun on a quiet machine\n", squeezeLevels[0])
		return
	}
	fmt.Printf("  minimum      %.2f cores (score %.0f), %.0f MB RAM for lerd\n", m["req.min_cpus"], m["req.min_cpu_score"], m["req.ram_mb"])
	if m["req.rec_cpus"] < 0 {
		fmt.Printf("  recommended  more than %g cores\n", squeezeLevels[0])
	} else {
		fmt.Printf("  recommended  %.2f cores (score %.0f)\n", m["req.rec_cpus"], m["req.rec_cpu_score"])
	}
	fmt.Println("  score = cores x single-core sha256 MB/s; divide by another CPU's calib.single_mbps for its cores")
}

// verdict calls a change real only when it beats 10%, twice the measured
// noise, and an absolute floor, so idle jitter on a fast machine is not news.
func verdict(key string, old, new, noise float64) string {
	if strings.HasPrefix(key, "calib.") || key == "containers.count" {
		return "" // the machine, not lerd
	}
	// A requirement of -1 means even the most CPUs failed, not a small number.
	if strings.HasPrefix(key, "req.") && (old < 0 || new < 0) {
		switch {
		case old >= 0 && new < 0:
			return "REGRESSED"
		case old < 0 && new >= 0:
			return "improved"
		}
		return ""
	}
	diff := new - old
	if math.Abs(diff) < absFloor(key) {
		return ""
	}
	// From zero there is no ratio to take; past the floor, the change is real.
	if old != 0 && math.Abs(diff)/old <= max(0.10, 2*noise) {
		return ""
	}
	if old == 0 && absFloor(key) == 0 {
		return ""
	}
	worse := diff > 0
	if strings.HasPrefix(key, "http.rps") {
		worse = diff < 0
	}
	if worse {
		return "REGRESSED"
	}
	return "improved"
}

func absFloor(key string) float64 {
	switch {
	case strings.Contains(key, "cpu_pct"):
		return 0.5
	case strings.Contains(key, "_mb"):
		return 5
	case strings.Contains(key, "_ms"):
		return 1
	case strings.HasSuffix(key, "_s"):
		return 0.5
	}
	return 0
}

// cleanups is a run's undo list: links and CPU limits add theirs as they are
// made. run takes them newest first, once; a second caller, such as Ctrl+C
// during the deferred run, waits for it instead of exiting mid-restore.
type cleanups struct {
	mu  sync.Mutex
	fns []func()
	ran bool
}

var cleanup cleanups

func (c *cleanups) add(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fns = append(c.fns, fn)
}

// guard runs an action and registers the undo it returns as one step, so a
// Ctrl+C landing mid-action waits for it and then undoes it.
func (c *cleanups) guard(action func() (undo func(), err error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ran {
		return errors.New("interrupted")
	}
	undo, err := action()
	if undo != nil {
		c.fns = append(c.fns, undo)
	}
	return err
}

func (c *cleanups) run() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ran {
		return
	}
	c.ran = true
	for i := len(c.fns) - 1; i >= 0; i-- {
		c.fns[i]()
	}
}

// delta formats the change from old to new as a signed percentage.
func delta(old, new float64) string {
	if old == 0 {
		if new == 0 {
			return "±0%"
		}
		return "new"
	}
	return fmt.Sprintf("%+.1f%%", (new-old)/old*100)
}

func lerdVersion() string {
	out, err := runCmd("lerd", "--version")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.TrimPrefix(out, "lerd version "))
}

// benchFixtures are the sites the bench links itself, under fixtureRoot.
var benchFixtures = map[string]bool{fixtureName: true, laravelFixture: true}

// fixtureRoot holds the fixture folders, a var so tests can move it.
var fixtureRoot = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Lerd")
}()

func fixtureDir(name string) string { return filepath.Join(fixtureRoot, name) }

// benchOwnedRow reports whether a `lerd sites` row is one of the bench's own
// fixtures: the name and the folder both match, so a user's site that only
// shares the name still counts as theirs.
func benchOwnedRow(line string) bool {
	f := strings.Fields(line)
	return len(f) > 0 && benchFixtures[f[0]] && strings.HasSuffix(strings.TrimSpace(line), fixtureDir(f[0]))
}

// userSiteCount counts the user's sites in `lerd sites`, without the header,
// alias lines or the bench's own fixtures.
func userSiteCount(sitesOutput string) int {
	n := 0
	for i, l := range strings.Split(strings.TrimSpace(sitesOutput), "\n") {
		f := strings.Fields(l)
		if i == 0 || len(f) == 0 || strings.HasPrefix(f[0], "↳") || benchOwnedRow(l) {
			continue
		}
		n++
	}
	return n
}

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func timeCmd(name string, args ...string) (time.Duration, error) {
	t := time.Now()
	_, err := runCmd(name, args...)
	return time.Since(t), err
}

// medianCmd runs a command n times and returns its median wall time in ms.
func medianCmd(n int, name string, args ...string) float64 {
	var ds []float64
	for i := 0; i < n; i++ {
		d, err := timeCmd(name, args...)
		if err != nil {
			return -1
		}
		ds = append(ds, float64(d.Microseconds())/1000)
	}
	return median(ds)
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}
