package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
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

// squeeze throttles the fixture's nginx and FPM containers step by step and
// records the fewest CPUs that still serve a scaffolded framework page in time.
func squeeze(m map[string]float64) error {
	url, unlink, err := linkLaravel()
	if err != nil {
		return err
	}
	defer unlink()

	ctrs, err := servingContainers(laravelFixture)
	if err != nil {
		return err
	}
	if want := "lerd-php" + strings.ReplaceAll(fixturePHP, ".", "") + "-fpm"; ctrs[1] != want {
		return fmt.Errorf("fixture is served by %s, expected %s", ctrs[1], want)
	}
	restore, err := limitable(ctrs)
	if err != nil {
		return err
	}
	defer restore()
	// A throttled container must never outlive the bench, so undo on Ctrl+C too.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; restore(); os.Exit(130) }()
	defer signal.Stop(sig)

	client := newClient()
	for i := 0; i < 10; i++ { // warm caches before the first level
		if err := get(client, url); err != nil {
			return err
		}
	}
	var peakMB float64
	var p95s []float64
	for _, cpus := range squeezeLevels {
		if err := setCPUs(ctrs, cpus); err != nil {
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

// linkLaravel scaffolds the framework fixture once, then links it for the run
// and unlinks it afterwards unless it was linked before the bench started.
func linkLaravel() (string, func(), error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Lerd", laravelFixture)
	url := "http://" + laravelFixture + ".test/"
	sites, _ := runCmd("lerd", "sites")
	wasLinked := strings.Contains(sites, laravelFixture+".test")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		step("scaffolding " + dir + " (once)")
		if _, err := runCmd("lerd", "new", dir, "--framework=laravel"); err != nil {
			return "", nil, fmt.Errorf("lerd new: %w", err)
		}
	}
	if now, _ := runCmd("lerd", "sites"); !strings.Contains(now, laravelFixture+".test") {
		if err := inDir(dir, "lerd", "link", "--yes"); err != nil {
			return "", nil, err
		}
	}
	// Pin a PHP version so the cap lands on an FPM container no worker shares.
	if err := inDir(dir, "lerd", "isolate", fixturePHP); err != nil {
		return "", nil, err
	}
	// Without a terminal lerd new stops after scaffolding, so finish it here once.
	if _, err := os.Stat(filepath.Join(dir, "vendor")); os.IsNotExist(err) {
		step("running lerd setup in " + dir + " (once)")
		if err := inDir(dir, "lerd", "setup", "--all", "--skip-open"); err != nil {
			return "", nil, err
		}
	}
	// Keep sessions and cache in memory so the squeeze measures CPU, not
	// SQLite fsyncs that stall whenever the disk is busy.
	if err := setEnv(filepath.Join(dir, ".env"), map[string]string{"SESSION_DRIVER": "array", "CACHE_STORE": "array"}); err != nil {
		return "", nil, err
	}
	undo := func() {}
	if !wasLinked {
		undo = func() {
			unlink := exec.Command("lerd", "unlink")
			unlink.Dir = dir
			if out, err := unlink.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "bench: lerd unlink: %v: %s\n", err, out)
			}
		}
	}
	if err := waitOK(newClient(), url, 60*time.Second); err != nil {
		undo()
		return "", nil, err
	}
	return url, undo, nil
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
	pid, err := runCmd("podman", "inspect", ctr, "--format", "{{.State.Pid}}")
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

// limitable records each container's cpu.max and returns the undo.
func limitable(ctrs []string) (func(), error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("-squeeze needs Linux cgroup v2, not %s", runtime.GOOS)
	}
	orig := map[string]string{}
	for _, c := range ctrs {
		f, err := cgroupCPUMax(c)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		orig[f] = strings.TrimSpace(string(b))
	}
	done := false
	return func() {
		if done {
			return
		}
		done = true
		for f, line := range orig {
			if err := os.WriteFile(f, []byte(line), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "bench: could not restore %s, run: echo %q > %s\n", f, line, f)
			}
		}
	}, nil
}

func setCPUs(ctrs []string, cpus float64) error {
	for _, c := range ctrs {
		f, err := cgroupCPUMax(c)
		if err != nil {
			return err
		}
		if err := os.WriteFile(f, []byte(cpuMaxLine(cpus)), 0o644); err != nil {
			return fmt.Errorf("limit %s: %w", c, err)
		}
	}
	return nil
}

// cpuMaxLine renders a CPU count as a cgroup v2 cpu.max line, 100ms period.
func cpuMaxLine(cpus float64) string {
	return fmt.Sprintf("%d 100000", int(cpus*100000))
}
