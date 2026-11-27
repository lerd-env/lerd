package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// A page a developer waits on: p95 under four parallel requests (a page and
// its assets) must stay under these for the level to count.
const (
	minP95ms = 250.0
	recP95ms = 100.0
)

var squeezeLevels = []float64{4, 2, 1, 0.5, 0.25, 0.1, 0.05}

const (
	laravelFixture = "bench-laravel"
	fixturePHP     = "8.4"
)

// squeezeShared allows squeezing a PHP container other sites also run on.
var squeezeShared bool

// squeeze throttles the fixture's nginx and FPM containers step by step and
// records the fewest CPUs that still serve a scaffolded framework page in time.
func squeeze(m map[string]float64) error {
	url, err := linkLaravel()
	if err != nil {
		return err
	}
	ctrs, err := servingContainers(laravelFixture)
	if err != nil {
		return err
	}
	if want := "lerd-php" + strings.ReplaceAll(fixturePHP, ".", "") + "-fpm"; ctrs[1] != want {
		return fmt.Errorf("fixture is served by %s, expected %s", ctrs[1], want)
	}
	// The cap lands on the whole container, so every site on it slows too and
	// their work skews the numbers.
	sites, _ := runCmd("lerd", "sites")
	if others := sharersOf(sites, fixturePHP, laravelFixture); len(others) > 0 && !squeezeShared {
		return fmt.Errorf("PHP %s is shared with %s; their work would skew the squeeze and they would be throttled too. Pass -squeeze-shared to go ahead", fixturePHP, strings.Join(others, ", "))
	}
	lim, err := newLimiter(ctrs)
	if err != nil {
		return err
	}
	cleanup.add(lim.restore)

	client := newClient()
	for i := 0; i < 10; i++ { // warm caches before the first level
		if err := get(client, url); err != nil {
			return err
		}
	}
	var peakMB float64
	var p95s []float64
	for _, cpus := range squeezeLevels {
		if err := lim.set(cpus); err != nil {
			return err
		}
		_, p95, err := load(client, url, 4, 10*time.Second)
		if err != nil {
			return err
		}
		usage, err := containerUsage()
		if err != nil {
			return err
		}
		var mb float64
		for _, c := range ctrs {
			mb += usage[c].memMB
		}
		peakMB = max(peakMB, mb)
		m[fmt.Sprintf("squeeze.p95_ms.cpus_%g", cpus)] = p95
		fmt.Fprintf(os.Stderr, "    %g CPUs: p95 %.0f ms\n", cpus, p95)
		p95s = append(p95s, p95)
	}
	lim.restore()
	m["req.min_cpus"] = lowestPassing(squeezeLevels, p95s, minP95ms)
	m["req.rec_cpus"] = lowestPassing(squeezeLevels, p95s, recP95ms)
	m["req.serving_mem_mb_peak"] = peakMB
	// RAM lerd needs for one site: its host processes (the tray is optional)
	// plus nginx and FPM at their peak under load.
	m["req.ram_mb"] = m["idle.rss_mb.host.ui"] + m["idle.rss_mb.host.watcher"] + m["idle.rss_mb.host.dns"] + peakMB
	// CPU in machine-independent units: cores times this CPU's single-core speed.
	m["req.min_cpu_score"] = m["req.min_cpus"] * m["calib.single_mbps"]
	m["req.rec_cpu_score"] = m["req.rec_cpus"] * m["calib.single_mbps"]
	return nil
}

// sharersOf names the other sites `lerd sites` lists on a PHP version.
func sharersOf(sitesOutput, php, self string) []string {
	var out []string
	for i, l := range strings.Split(sitesOutput, "\n") {
		f := strings.Fields(l)
		if i == 0 || len(f) < 3 || strings.HasPrefix(f[0], "↳") || f[0] == self {
			continue
		}
		if f[2] == php {
			out = append(out, f[0])
		}
	}
	return out
}

// lowestPassing walks the levels from most to fewest CPUs and returns the last
// one before the first miss, so a lucky pass below a failure does not count.
// It returns -1 when even the first level misses.
func lowestPassing(levels, p95s []float64, limit float64) float64 {
	best := -1.0
	for i, p := range p95s {
		if p > limit {
			break
		}
		best = levels[i]
	}
	return best
}

// linkLaravel scaffolds the framework fixture once, links it for the run and
// returns its URL; the unlink is registered the moment the link exists.
func linkLaravel() (string, error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Lerd", laravelFixture)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		step("scaffolding " + dir + " (once)")
		// lerd new links what it scaffolds, so this run owns that link too.
		if err := cleanup.guard(func() (func(), error) {
			if _, err := runCmd("lerd", "new", dir, "--framework=laravel"); err != nil {
				return unlinkLater(dir), fmt.Errorf("lerd new: %w", err)
			}
			return unlinkLater(dir), nil
		}); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, fixtureMarker), nil, 0o644); err != nil {
			return "", err
		}
	} else if err := claimFixtureDir(dir); err != nil {
		return "", err
	}
	if err := linkSite(laravelFixture, dir); err != nil {
		return "", err
	}
	// Pin a PHP version so the cap lands on an FPM container no worker shares.
	if err := inDir(dir, "lerd", "isolate", fixturePHP); err != nil {
		return "", err
	}
	// Without a terminal lerd new stops after scaffolding, so finish it here once.
	if _, err := os.Stat(filepath.Join(dir, "vendor")); os.IsNotExist(err) {
		step("running lerd setup in " + dir + " (once)")
		if err := inDir(dir, "lerd", "setup", "--all", "--skip-open"); err != nil {
			return "", err
		}
	}
	// Keep sessions and cache in memory so the squeeze measures CPU, not
	// SQLite fsyncs that stall whenever the disk is busy.
	if err := setEnv(filepath.Join(dir, ".env"), map[string]string{"SESSION_DRIVER": "array", "CACHE_STORE": "array"}); err != nil {
		return "", err
	}
	url, err := siteURL(laravelFixture)
	if err != nil {
		return "", err
	}
	return url, waitOK(newClient(), url, 60*time.Second)
}

// setEnv rewrites the given keys in a dotenv file, appending any that are missing.
func setEnv(path string, kv map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	seen := map[string]bool{}
	for i, l := range lines {
		k, _, ok := strings.Cut(l, "=")
		if v, want := kv[k]; ok && want {
			lines[i], seen[k] = k+"="+v, true
		}
	}
	for k, v := range kv {
		if !seen[k] {
			lines = append(lines, k+"="+v)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func inDir(dir, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// servingContainers returns nginx and the FPM container of the site's PHP version.
func servingContainers(site string) ([]string, error) {
	out, err := runCmd("lerd", "sites")
	if err != nil {
		return nil, err
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == site {
			return []string{"lerd-nginx", "lerd-php" + strings.ReplaceAll(f[2], ".", "") + "-fpm"}, nil
		}
	}
	return nil, fmt.Errorf("site %s not in lerd sites", site)
}

// cgroupCPUMax returns the cpu.max file of a running container's cgroup.
// podman update --cpus cannot lift a limit again (0 is ignored), so the bench
// writes cgroup v2 directly and puts back the exact original line.
func cgroupCPUMax(ctr string) (string, error) {
	pid, err := podman.Run("inspect", ctr, "--format", "{{.State.Pid}}")
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile("/proc/" + strings.TrimSpace(pid) + "/cgroup")
	if err != nil {
		return "", fmt.Errorf("%s cgroup: %w", ctr, err)
	}
	_, path, ok := strings.Cut(strings.TrimSpace(string(b)), "::")
	if !ok {
		return "", fmt.Errorf("%s is not on cgroup v2: %q", ctr, b)
	}
	return filepath.Join("/sys/fs/cgroup", path, "cpu.max"), nil
}

// limiter caps containers through cgroup v2 and puts their original cpu.max
// back. Both hold the lock, so a restore waits for a write under way and no
// write lands after it.
type limiter struct {
	mu       sync.Mutex
	ctrs     []string
	orig     map[string]string
	restored bool
}

func newLimiter(ctrs []string) (*limiter, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("-squeeze needs Linux cgroup v2, not %s", runtime.GOOS)
	}
	l := &limiter{ctrs: ctrs, orig: map[string]string{}}
	for _, c := range ctrs {
		f, err := cgroupCPUMax(c)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		l.orig[f] = strings.TrimSpace(string(b))
	}
	return l, nil
}

func (l *limiter) set(cpus float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.restored {
		return fmt.Errorf("limits already restored")
	}
	for f := range l.orig {
		if err := os.WriteFile(f, []byte(cpuMaxLine(cpus)), 0o644); err != nil {
			return fmt.Errorf("limit %s: %w", f, err)
		}
	}
	return nil
}

func (l *limiter) restore() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.restored {
		return
	}
	l.restored = true
	for f, line := range l.orig {
		if err := os.WriteFile(f, []byte(line), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "bench: could not restore %s, run: echo %q > %s\n", f, line, f)
		}
	}
}

// cpuMaxLine renders a CPU count as a cgroup v2 cpu.max line, 100ms period.
func cpuMaxLine(cpus float64) string {
	return fmt.Sprintf("%d 100000", int(cpus*100000))
}
