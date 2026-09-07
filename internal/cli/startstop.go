package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/imagepull"
	"github.com/geodro/lerd/internal/lifecycle"
	"github.com/geodro/lerd/internal/nginx"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/services"
	"github.com/geodro/lerd/internal/shims"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// quadletImage reads the Image= value from an installed quadlet file.
// Returns "" if the file cannot be read or has no Image= line.
func quadletImage(unit string) string {
	path := filepath.Join(config.QuadletDir(), unit+".container")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if after, ok := strings.CutPrefix(line, "Image="); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// imageWork pairs the job that produces an image with the disclosure of what
// that job downloads, so a new case can never add a silent download.
type imageWork struct {
	job  BuildJob
	item imagepull.Item
}

// ensureImages checks all images required by units that are about to start,
// discloses everything it is about to download, and then builds or pulls any
// that are missing using the parallel spinner UI.
func ensureImages() {
	work := pendingImageWork()
	if len(work) == 0 {
		return
	}
	plan := make(imagepull.Plan, len(work))
	jobs := make([]BuildJob, len(work))
	for i, w := range work {
		plan[i], jobs[i] = w.item, w.job
	}
	plan.Fill().Report(os.Stdout)
	if imagepull.DryRun() {
		return
	}
	RunParallel(jobs) //nolint:errcheck
}

// pendingImageWork lists every image a start would have to build or pull
// because it is not in the local store.
func pendingImageWork() []imageWork {
	units := append(lifecycle.CoreUnits(), lifecycle.InstalledServiceUnits()...)
	units = append(units, lifecycle.InstalledCustomContainerUnits()...)
	var work []imageWork
	seen := map[string]bool{}

	for _, unit := range units {
		image := quadletImage(unit)

		// On macOS there are no quadlet files, so quadletImage returns "".
		// Derive the image name from the unit name for PHP-FPM units so that
		// images are rebuilt after a VM reset without requiring manual intervention.
		if image == "" && strings.HasPrefix(unit, "lerd-php") && strings.HasSuffix(unit, "-fpm") {
			short := strings.TrimSuffix(strings.TrimPrefix(unit, "lerd-php"), "-fpm")
			image = "lerd-php" + short + "-fpm:local"
		}

		if image == "" || seen[image] {
			continue
		}
		seen[image] = true

		if podman.RunSilent("image", "exists", image) == nil {
			continue // already present
		}

		img := image
		reason := "missing, needed by " + strings.TrimPrefix(unit, "lerd-")
		switch {
		case img == podman.DNSMasqImage:
			work = append(work, imageWork{
				job: BuildJob{
					Label: "Building dnsmasq",
					Run: func(w io.Writer) error {
						return podman.BuildDNSMasqImage(w, dns.ReadUpstreamDNS())
					},
				},
				item: imagepull.Build("dnsmasq image", podman.DNSMasqBaseImage, reason),
			})

		case strings.HasPrefix(img, "lerd-php") && strings.HasSuffix(img, "-fpm:local"):
			// Extract version from image name, e.g. lerd-php84-fpm:local → 8.4
			short := strings.TrimSuffix(strings.TrimPrefix(img, "lerd-php"), "-fpm:local")
			ver := short[:1] + "." + short[1:]
			v := ver
			work = append(work, imageWork{
				job: BuildJob{
					Label: "PHP " + v,
					Run: func(w io.Writer) error {
						_, err := podman.BuildFPMImageTo(v, false, w)
						return err
					},
				},
				item: imagepull.Build("PHP "+v+" image", podman.PHPBaseImageRef(v), reason),
			})

		case strings.HasPrefix(img, "localhost/lerd-frankenphp") && strings.HasSuffix(img, ":local"):
			// Build the derived FrankenPHP image, e.g.
			// localhost/lerd-frankenphp84:local → 8.4
			short := strings.TrimSuffix(strings.TrimPrefix(img, "localhost/lerd-frankenphp"), ":local")
			if len(short) < 2 {
				continue // malformed tag with no version digits; skip rather than panic
			}
			v := short[:1] + "." + short[1:]
			work = append(work, imageWork{
				job: BuildJob{
					Label: "FrankenPHP " + v,
					Run:   func(w io.Writer) error { return podman.BuildFrankenPHPImage(v, false, w) },
				},
				item: imagepull.Build("FrankenPHP "+v+" image", podman.FrankenPHPBaseImage(v), reason),
			})

		case strings.HasPrefix(img, "lerd-custom-") && strings.HasSuffix(img, ":local"):
			// Rebuild custom container from the site's Containerfile.
			siteName := strings.TrimSuffix(strings.TrimPrefix(img, "lerd-custom-"), ":local")
			sn := siteName
			work = append(work, imageWork{
				job: BuildJob{
					Label: "Custom: " + sn,
					Run: func(w io.Writer) error {
						site, err := config.FindSite(sn)
						if err != nil {
							return err
						}
						proj, err := config.LoadProjectConfig(site.Path)
						if err != nil {
							return err
						}
						return podman.BuildCustomImageTo(sn, site.Path, proj.Container, w)
					},
				},
				// The site's own Containerfile decides what this downloads, so
				// there is no single base image to size up.
				item: imagepull.Build("Custom container: "+sn, "", reason),
			})

		default:
			label := podman.PlatformImage(img)
			work = append(work, imageWork{
				job: BuildJob{
					Label: "Pulling " + label,
					Run: func(w io.Writer) error {
						return podman.PullImageTo(label, w)
					},
				},
				item: imagepull.Pull(label, reason),
			})
		}
	}

	return work
}

// NewStartCmd returns the start command.
func NewStartCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start Lerd (DNS, nginx, PHP-FPM, and installed services)",
		RunE: func(*cobra.Command, []string) error {
			if dryRun {
				imagepull.SetDryRun(true)
				ReportPendingDownloads(os.Stdout)
				return nil
			}
			return startLerd(nil, nil)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Report the images a start would pull or rebuild, with their sizes, and exit")
	return cmd
}

// ReportPendingDownloads discloses everything a start would download without
// downloading or starting anything.
func ReportPendingDownloads(w io.Writer) {
	work := pendingImageWork()
	if len(work) == 0 {
		feedback.LineOn(w, "Nothing to download: every image lerd needs is already in the local store.")
		return
	}
	plan := make(imagepull.Plan, len(work))
	for i, it := range work {
		plan[i] = it.item
	}
	plan.Fill().Report(w)
}

// NewStopCmd returns the stop command.
func NewStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop Lerd containers (DNS, nginx, PHP-FPM, and running services)",
		RunE:  runStop,
	}
}

// NewQuitCmd returns the quit command.
func NewQuitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quit",
		Short: "Stop all Lerd processes and containers (including UI, watcher, and tray)",
		RunE:  runQuit,
	}
}

// ensureDefaultPHPInstalled builds the FPM image and writes the unit file for
// the configured default PHP version if it has never been installed. This
// handles the case where the user sets a new default (e.g. 8.5) before running
// `lerd php install`, so `lerd start` transparently installs it.
func ensureDefaultPHPInstalled() {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil || cfg.PHP.DefaultVersion == "" {
		return
	}
	defaultVer := cfg.PHP.DefaultVersion
	installed, _ := phpPkg.ListInstalled()
	for _, v := range installed {
		if v == defaultVer {
			return // already installed
		}
	}
	fmt.Printf("  --> Installing PHP %s (configured default, not yet installed) ...\n", defaultVer)
	if err := podman.BuildFPMImage(defaultVer, false); err != nil {
		fmt.Printf("  WARN: build PHP %s image: %v\n", defaultVer, err)
		return
	}
	if err := podman.WriteFPMQuadlet(defaultVer); err != nil {
		fmt.Printf("  WARN: write PHP %s unit: %v\n", defaultVer, err)
	}
}

// PortCheck pairs a host port with a human-readable label and container name.
type PortCheck struct {
	Port      string // host port number
	Label     string // e.g. "nginx HTTP", "mysql"
	Container string // lerd container name
}

// builtinExtraPorts lists secondary host ports for built-in services that are
// hardcoded in the quadlet files but not reflected in config.ServiceConfig.Port.
var builtinExtraPorts = map[string][]string{
	"rustfs":  {"9001"},
	"mailpit": {"8025"},
}

// hostPort extracts the host port from a port mapping string ("host:container").
// If no colon is present the whole string is returned.
func hostPort(mapping string) string {
	if i := strings.Index(mapping, ":"); i >= 0 {
		return mapping[:i]
	}
	return mapping
}

// CollectPortChecks builds the list of ports to verify for the given units.
func CollectPortChecks(units []string) []PortCheck {
	unitSet := make(map[string]bool, len(units))
	for _, u := range units {
		unitSet[u] = true
	}

	var checks []PortCheck

	// Nginx ports (configurable).
	if unitSet["lerd-nginx"] {
		httpPort, httpsPort := config.NginxPorts()
		checks = append(checks,
			PortCheck{strconv.Itoa(httpPort), "nginx HTTP", "lerd-nginx"},
			PortCheck{strconv.Itoa(httpsPort), "nginx HTTPS", "lerd-nginx"},
		)
	}

	// DNS port.
	if unitSet["lerd-dns"] {
		checks = append(checks, PortCheck{"5300", "dns", "lerd-dns"})
	}

	// Built-in services.
	cfg, _ := config.LoadGlobal()
	for _, svc := range knownServices() {
		if !unitSet["lerd-"+svc] {
			continue
		}
		container := "lerd-" + svc
		if cfg != nil {
			if sc, ok := cfg.Services[svc]; ok {
				// A PublishedPort override moves the primary published port, so
				// check the real bound port rather than the preset default.
				port := sc.Port
				if sc.PublishedPort > 0 {
					port = sc.PublishedPort
				}
				if port > 0 {
					checks = append(checks, PortCheck{strconv.Itoa(port), svc, container})
				}
				for _, ep := range sc.ExtraPorts {
					checks = append(checks, PortCheck{hostPort(ep), svc, container})
				}
			}
		}
		for _, ep := range builtinExtraPorts[svc] {
			checks = append(checks, PortCheck{ep, svc, container})
		}
	}

	// Custom services.
	customs, _ := config.ListCustomServices()
	for _, svc := range customs {
		if !unitSet["lerd-"+svc.Name] {
			continue
		}
		container := "lerd-" + svc.Name
		for _, p := range svc.Ports {
			checks = append(checks, PortCheck{hostPort(p), svc.Name, container})
		}
	}

	return checks
}

// checkPortConflicts warns about ports already in use by non-lerd processes.
func checkPortConflicts(units []string) {
	checks := CollectPortChecks(units)
	if len(checks) == 0 {
		return
	}

	ss := PortListOutput()
	if ss == "" {
		return
	}

	var conflicts []string
	for _, c := range checks {
		if isPortConflict(c, ss, podmanContainerRunning, lerdDNSAnswering) {
			conflicts = append(conflicts,
				fmt.Sprintf("  WARN: port %s (%s) already in use, may fail to start (check: %s)", c.Port, c.Label, FindListenerCmd(c.Port)))
		}
	}
	if len(conflicts) > 0 {
		fmt.Println("Port conflicts detected:")
		for _, msg := range conflicts {
			fmt.Println(msg)
		}
		fmt.Println()
	}
}

// isPortConflict reports whether a port check is a genuine clash with a foreign
// process. A lerd service that already owns its port is never a conflict, in
// three ways: a running container owns it directly; lerd-dns owns it when its
// own dnsmasq is already answering; and on macOS the podman machine's gvproxy
// owns any published port by forwarding it into the VM.
//
// The dnsmasq case matters because on macOS lerd-dns runs as a launchd-managed
// dnsmasq process, not a podman container, so containerRunning is always false
// for it; without the dnsAnswering guard the still-listening dnsmasq from the
// previous session looks like a foreign conflict and mis-fires the "port 5300
// already in use" warning on every `lerd start`. The gvproxy case matters
// because lerd's service containers never bind host ports directly on macOS
// (no -p in their plists); host reachability comes from gvproxy forwarding into
// the VM, so a gvproxy-held service port is lerd's own forward from a prior
// session, not a foreign process. The func seams keep this pure and unit-testable.
func isPortConflict(c PortCheck, portList string, containerRunning func(string) bool, dnsAnswering func() bool) bool {
	if containerRunning(c.Container) {
		return false
	}
	if c.Container == "lerd-dns" && dnsAnswering() {
		return false
	}
	if !PortInUseIn(c.Port, portList) {
		return false
	}
	return !portOwnedByMachineProxy(c.Port, portList)
}

// portOwnedByMachineProxy reports whether the listener on the given port is the
// podman machine's gvproxy. On macOS that proxy owns every published host port
// (lerd's containers themselves carry no -p), so a gvproxy-held port is a
// lerd/podman forward into the VM rather than a foreign blocker. On Linux there
// is no gvproxy, so this never matches and the check is a harmless no-op.
func portOwnedByMachineProxy(port, portList string) bool {
	for _, line := range strings.Split(portList, "\n") {
		if strings.HasPrefix(line, "gvproxy") && strings.Contains(line, ":"+port+" ") {
			return true
		}
	}
	return false
}

// podmanContainerRunning adapts podman.ContainerRunning to the bool-only seam
// isPortConflict expects, treating a probe error as "not running".
func podmanContainerRunning(name string) bool {
	running, _ := podman.ContainerRunning(name)
	return running
}

// lerdDNSAnswering reports whether lerd's own dnsmasq is currently answering for
// the configured TLD, which means a listener on the DNS port is lerd-dns itself
// rather than a foreign process.
func lerdDNSAnswering() bool {
	return dns.CheckStatus(dns.ConfiguredTLD()) != dns.StatusDown
}

// StartEvent is one step of the start sequence. The dashboard streams these so
// a start driven from the UI reads like the CLI's spinner instead of a button
// that hangs for a minute with nothing to show for it.
type StartEvent struct {
	// Phase is "step" for a stage of the sequence, "unit" for one unit that has
	// finished starting (Error set if it failed), and "done" at the end.
	Phase string `json:"phase"`
	Step  string `json:"step,omitempty"`
	Unit  string `json:"unit,omitempty"`
	// Total is the number of units the run will start, sent once before the
	// first of them so the dashboard can show progress out of a known total.
	Total int    `json:"total,omitempty"`
	Error string `json:"error,omitempty"`
}

// startLerd is the full start sequence. emit, when set, receives a StartEvent
// per stage and per unit; it is called from the parallel start jobs, so it is
// serialised here rather than in every caller.
//
// skip names units the run must not touch, for a daemon starting lerd from
// inside one of them: starting a unit boots it out of launchd first, which is
// the same SIGTERM a stop is, so lerd-ui asking for its own unit killed the
// start half way and left the dashboard unreachable behind a 502.
func startLerd(emit func(StartEvent), skip []string) error {
	var emitMu sync.Mutex
	report := func(e StartEvent) {
		if emit == nil {
			return
		}
		emitMu.Lock()
		defer emitMu.Unlock()
		emit(e)
	}
	report(StartEvent{Phase: "step", Step: "preparing"})
	// Clear the intentional-stop marker up front: we're bringing lerd up, so the
	// worker health watcher should resume reporting real drift once units are back.
	_ = config.ClearStopped()

	// Pre-ensure LastUp lets healMachineRestartIfNeeded distinguish an
	// external podman-machine restart (which orphans gvproxy port forwards)
	// from a stop+start the ensure itself performs. No-op on Linux.
	preEnsureLastUp := currentMachineLastUp()
	if err := ensurePodmanMachineRunning(); err != nil {
		return err
	}
	migrateExecWorkerPlists()
	healMachineRestartIfNeeded(preEnsureLastUp)

	// Podman orders every rootless quadlet after its network-online wait unit.
	// Where network-online.target never activates (Fedora Silverblue and other
	// atomic images) that unit only ever times out, so each container start,
	// and the boot itself, stalls for 90s. Override it before starting anything.
	if applied, err := lerdSystemd.EnsureNoNetworkWaitStall(); err != nil {
		fmt.Printf("  WARN: skip podman network-online wait: %v\n", err)
	} else if applied {
		fmt.Println("  Skipping podman's network-online wait (this host never reaches that target)")
	}

	// Self-heal a podman upgrade before touching the network or starting
	// containers. A major-version or backend change since the last run
	// reshuffles rootless storage/networking and otherwise surfaces as the
	// cryptic "rootless netns" container start failure (#635). No-op unless
	// drift is detected. The heal force-removes the lerd containers; the start
	// sequence below brings them back up, so the returned list is not needed
	// here.
	containerDNS := dns.ReadContainerDNS()
	_ = healPodmanUpgrade(containerDNS)

	// Ensure the lerd bridge network exists. On macOS the network is stored
	// inside the Podman Machine VM; it may be absent after a fresh machine
	// init or if it was pruned. All service containers use --network lerd so
	// this must succeed before any container is started.
	if err := podman.EnsureNetwork("lerd", containerDNS); err != nil {
		if errors.Is(err, podman.ErrNetworkNeedsMigration) {
			fmt.Println("  WARN: lerd network schema doesn't match host IPv6 support; run 'lerd install' to recreate")
		} else {
			fmt.Printf("  WARN: ensure lerd network: %v\n", err)
		}
	}

	// Repair units and shims left pointing at a lerd binary that moved, which
	// is what a package-manager upgrade of lerd itself does to an install.
	if units, shims := healLerdBinaryMove(); len(units)+len(shims) > 0 {
		fmt.Println("  " + repairSummary(units, shims))
	}

	// Restore quadlets and worker units that may be missing after an
	// uninstall/reinstall cycle. Reads .lerd.yaml from each active site.
	restoreSiteInfrastructure()

	// Reconcile custom services against their YAMLs (issue #678): regenerate a
	// missing quadlet, drop an orphan quadlet with no YAML. Data dirs untouched.
	reconcileCustomServices()

	// Heal quadlets whose IPv6 publish lines the host can no longer bind. A
	// VPN client that turns IPv6 off host-wide takes ::1 with it, and every
	// unit still carrying a [::1] line dies with exit 126 (#1634).
	if healed, err := podman.HealIPv6Binds(); err != nil {
		fmt.Printf("  WARN: healing IPv6 binds: %v\n", err)
	} else if len(healed) > 0 {
		fmt.Printf("  Rewrote %d unit(s) to match the host's IPv6 support\n", len(healed))
		_ = podman.DaemonReloadFn()
		for _, name := range healed {
			if status, _ := services.Mgr.UnitStatus(name); status == "active" || status == "activating" {
				if err := podman.RestartUnit(name); err != nil {
					fmt.Printf("  WARN: restarting %s: %v\n", name, err)
				}
			}
		}
	}

	// If the configured default PHP version has never been installed (no plist /
	// quadlet / container), install it now so CoreUnits can include it.
	ensureDefaultPHPInstalled()

	// Pre-flight port conflict check.
	units := append(lifecycle.CoreUnits(), lifecycle.InstalledServiceUnits()...)
	checkPortConflicts(units)

	// Build or pull any missing images before starting containers.
	report(StartEvent{Phase: "step", Step: "images"})
	ensureImages()

	// Rewrite nginx.conf so any config changes in new binary versions take effect.
	if err := nginx.EnsureNginxConfig(); err != nil {
		fmt.Printf("  WARN: nginx config: %v\n", err)
	}
	// The quadlet carries the host ports nginx publishes, so a start has to
	// rewrite it too, and restart the container when it changed: a running
	// nginx keeps the mapping it was created with, so writing the unit alone
	// leaves a moved nginx.http_port unapplied until something else restarts
	// it (#1544).
	if quadletChanged, err := nginx.RewriteNginxQuadlet(); err != nil {
		fmt.Printf("  WARN: nginx quadlet: %v\n", err)
	} else if quadletChanged {
		_ = podman.DaemonReloadFn()
		if err := podman.RestartUnit("lerd-nginx"); err != nil {
			fmt.Printf("  WARN: restarting nginx on the new ports: %v\n", err)
		}
	}
	if err := nginx.EnsureLerdVhost(); err != nil {
		fmt.Printf("  WARN: lerd vhost: %v\n", err)
	}
	if err := nginx.EnsureProfilerVhost(); err != nil {
		fmt.Printf("  WARN: profiler vhost: %v\n", err)
	}
	// The lerd-nginx quadlet bind-mounts RunDir so the lerd.localhost vhost
	// can reach lerd-ui over a unix socket. The directory must exist before
	// the container starts or podman will create it root-owned.
	if err := os.MkdirAll(config.RunDir(), 0755); err != nil {
		fmt.Printf("  WARN: run dir: %v\n", err)
	}

	// Refresh dnsmasq upstream config from the current system DNS before lerd-dns starts.
	// This ensures the config reflects any DNS changes (new servers added, DHCP change)
	// that occurred since the last run without requiring a full reinstall.
	if err := dns.WriteDnsmasqConfig(config.DnsmasqDir()); err != nil {
		fmt.Printf("  WARN: dns config: %v\n", err)
	}

	// Write the shared hosts file mounted into PHP containers at /etc/hosts.
	if err := podman.WriteContainerHosts(); err != nil {
		fmt.Printf("  WARN: container hosts file: %v\n", err)
	}

	// Pre-flight: drop bind mounts whose host directory has gone (a branch
	// checkout that removed it, a deleted project). Podman refuses to start a
	// container with a missing bind source, so one such path otherwise takes
	// nginx and every site down with it (#1083).
	for _, r := range podman.RepairMissingMounts() {
		if r.Site != "" {
			fmt.Printf("  WARN: %s no longer exists (site %s), removed from %s\n", r.Path, r.Site, r.Unit)
		} else {
			fmt.Printf("  WARN: %s no longer exists, removed from %s\n", r.Path, r.Unit)
		}
	}

	// A service domain is served by no site, so nothing else rebuilds its vhost
	// or renews its certificate; do it before the repair sweep so a cert that
	// aged out is reissued rather than found missing.
	if err := serviceops.ApplyServiceDomains(); err != nil {
		fmt.Printf("  WARN: %v\n", err)
	}

	// Pre-flight: repair SSL vhosts with missing cert files so nginx can start.
	if repairs := nginx.RepairVhosts(); len(repairs) > 0 {
		for _, r := range repairs {
			switch r.Reason {
			case "missing-cert":
				fmt.Printf("  WARN: missing TLS certificate for %s — switched to HTTP\n", r.Domain)
			case "orphan-ssl":
				fmt.Printf("  WARN: removed orphan SSL vhost for %s\n", r.Domain)
			}
		}
	}

	// Reload nginx if it is already running so regenerated base vhosts (the
	// dashboard and profiler vhosts) take effect without a full restart.
	if running, _ := podman.ContainerRunning("lerd-nginx"); running {
		_ = nginx.Reload()
	}

	// Phase 1: start all infrastructure (containers, FPM, custom containers,
	// UI, watcher) before workers. Workers exec into containers, so they must
	// be up first.
	serviceUnits := append(lifecycle.CoreUnits(), lifecycle.InstalledServiceUnits()...)
	serviceUnits = append(serviceUnits, lifecycle.InstalledCustomContainerUnits()...)
	serviceUnits = append(serviceUnits, "lerd-ui", "lerd-watcher")
	serviceUnits = dropSkipped(serviceUnits, skip)

	// Phase 2: worker units that depend on running containers.
	workerUnits := append(lifecycle.RegisteredQueueUnits(), lifecycle.RegisteredStripeUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredScheduleUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredReverbUnits()...)
	// Also include non-standard framework workers (horizon, vite-dev, etc.)
	// declared in the site registry, so restored unit files get started here
	// rather than waiting for the next session.
	workerUnits = append(workerUnits, lifecycle.RegisteredFrameworkWorkerUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredTimerUnits()...)
	workerUnits = collapseTimerSiblings(dedupeStrings(workerUnits))
	// Don't resurrect workers the idle engine has gracefully suspended. Without
	// this, a boot or a manual start after stop would start a deliberately-asleep
	// worker while the registry still records it suspended, drifting the dashboard
	// (site shown asleep, workers actually running) and making workerheal skip it.
	// Mirrors the worktree autostart filter; real activity wakes it via the engine.
	workerUnits = dropIdleSuspendedUnits(workerUnits)
	workerUnits = dropSkipped(workerUnits, skip)

	feedback.Begin()
	feedback.Line("starting lerd")
	report(StartEvent{Phase: "step", Step: "units", Total: len(serviceUnits) + len(workerUnits)})

	makeJobs := func(us []string) []BuildJob {
		jobs := make([]BuildJob, len(us))
		for i, u := range us {
			unit := u
			label := strings.TrimSuffix(strings.TrimPrefix(unit, "lerd-"), ".timer")
			jobs[i] = BuildJob{
				Label: label,
				Run: func(w io.Writer) error {
					var err error
					if unit == "lerd-dns" {
						err = podman.RestartUnit(unit)
					} else {
						err = podman.StartUnit(unit)
					}
					ev := StartEvent{Phase: "unit", Unit: label}
					if err != nil {
						ev.Error = err.Error()
					}
					report(ev)
					return err
				},
			}
		}
		return jobs
	}

	startedServiceUnits := lifecycle.InstalledServiceUnits()
	serviceErr := RunParallel(makeJobs(serviceUnits))
	// When the Podman Machine's container storage is left corrupt after an
	// unclean host shutdown, every container start fails. Remount storage and
	// rebuild the stale containers (data is host bind-mounted, so this is safe),
	// then retry the start pass once. A ghost container (libpod DB entry intact
	// but its storage layer gone) is the other unclean-shutdown failure; purge it
	// inside the VM and retry the same way. The two signatures are exclusive.
	if healOverlayCorruptionIfNeeded(serviceErr) || healGhostContainersIfNeeded(serviceErr) {
		serviceErr = RunParallel(makeJobs(serviceUnits))
	}
	// Bulk start does not go through lerd service start, so discover_family
	// consumers (phpMyAdmin, pgAdmin) never got a post-engine regen. Reconcile
	// may also have written empty host lists before any engine was up. Refresh
	// once engines are running so PMA_HOSTS / LERD_POSTGRES_HOSTS match reality.
	serviceops.RefreshDiscoverFamilyConsumers()
	// systemd reports a unit active as soon as its container starts, but the
	// engine inside is not accepting connections yet. Without this wait, start
	// hands back control while mysql is still booting and the first request to
	// a database-backed site returns 500 until the engine catches up.
	waitServicesReady(startedServiceUnits, serviceReadyTimeout)
	// If the storage is still corrupt the heal couldn't fix it; every worker
	// (and the DNS and tray steps below) would fail the same way and bury the
	// recovery guidance. reportOverlayHealOutcome prints the guidance and
	// reports true only on the platform where this error occurs (macOS), so we
	// stop there; on every other platform it is a no-op that returns false and
	// the start continues as normal.
	if reportOverlayHealOutcome(serviceErr) {
		report(StartEvent{Phase: "done"})
		return nil
	}
	if len(workerUnits) > 0 {
		RunParallel(makeJobs(workerUnits)) //nolint:errcheck
	}

	// Regenerate the browser-testing hosts file now that nginx has its IP.
	// The file was written earlier with a possibly stale address; update it
	// so containers like Selenium resolve .test domains to the current
	// lerd-nginx container IP.
	if err := podman.WriteContainerHosts(); err != nil {
		fmt.Printf("  WARN: browser hosts file: %v\n", err)
	}

	// A service whose URLs reach a browser is broken until it has a name both
	// sides resolve, so the domain its preset declares is taken here rather than
	// waiting for the user to find a command. Runs after the services are up
	// because the env sweep that follows provisions against them.
	adoptDefaultServiceDomains()

	// Sync the pasta DNS proxy (169.254.1.1) as the aardvark-dns upstream for the lerd
	// network. This address chains through systemd-resolved, which resolves both .test
	// domains (via lerd-dns) and internet domains. Using 169.254.1.1 instead of the
	// host's real upstream avoids NXDOMAIN for .test while retaining internet access.
	if err := podman.EnsureNetworkDNS("lerd", dns.ReadContainerDNS()); err != nil {
		fmt.Printf("  WARN: network DNS: %v\n", err)
	}

	// Wait for lerd-dns to be ready before configuring the resolver.
	// systemctl start returns when the unit is active, but dnsmasq inside the
	// container may not be listening yet. If we set resolvectl to use port 5300
	// before it's up, systemd-resolved marks it failed and falls back to the
	// upstream DNS server, breaking .test resolution until manually fixed.
	if err := dns.WaitReady(10 * time.Second); err != nil {
		fmt.Printf("  WARN: %v\n", err)
	}

	// Refresh the sudoers drop-in before reapplying DNS config, but only where a
	// password prompt can be answered. A release that adds a privileged step ships
	// new grants, and writing /etc/sudoers.d/lerd needs a real authentication:
	// granting `tee` on sudoers.d would itself be an escalation, so it can never be
	// passwordless. Headless (lerd-ui driving a start), sudo has no tty and the
	// write just fails, so we skip it and let ConfigureResolver report what is
	// missing rather than burying a prompt no one can see. Content-hashed, so on an
	// unchanged drop-in this is a no-op either way.
	if dnsEnabled() && canPromptForPassword() {
		if err := dns.InstallSudoers(); err != nil {
			fmt.Printf("  WARN: refreshing DNS sudoers rule: %v\n", err)
		}
	}

	// Re-apply DNS routing so .test resolves via lerd-dns on every start.
	// resolvectl settings are ephemeral and reset on reboot; the NM dispatcher
	// script fires on interface "up" but that event precedes lerd-dns starting.
	// The NixOS note lives here (and on install), not inside ConfigureResolver:
	// the watcher calls that whenever .test fails.
	report(StartEvent{Phase: "step", Step: "dns"})
	dns.NoteNixOSOwnsResolver()
	if err := dns.ConfigureResolver(); err != nil {
		fmt.Printf("  WARN: DNS resolver config: %v\n", err)
	}

	autoStopUnusedFPMs()

	// Both launch paths below bring the tray back, which is why masking the
	// unit never kept it away; the preference has to be checked here instead.
	if trayEnabled() {
		tray := feedback.Start("starting lerd-tray")
		if err := launchTray(); err != nil {
			tray.Fail(err)
		} else {
			tray.OK("")
		}
	}

	report(StartEvent{Phase: "done"})
	return nil
}

// startRestoredServices pulls images and starts service units that have a quadlet
// installed but are not yet running. Called from lerd install to bring back services
// (mysql, redis, etc.) that were restored from .lerd.yaml.
func startRestoredServices() {
	units := lifecycle.InstalledServiceUnits()
	if len(units) == 0 {
		return
	}

	// Pull missing images first, disclosing the download before it starts.
	var pullJobs []BuildJob
	var plan imagepull.Plan
	seen := map[string]bool{}
	for _, unit := range units {
		// PlatformImage covers a quadlet still on the upstream image from before
		// the rewrite landed (idempotent once the unit is rewritten on start).
		image := podman.PlatformImage(quadletImage(unit))
		if image == "" || seen[image] {
			continue
		}
		seen[image] = true
		if podman.RunSilent("image", "exists", image) == nil {
			continue
		}
		img := image
		plan = append(plan, imagepull.Pull(img, "missing, needed by "+strings.TrimPrefix(unit, "lerd-")))
		pullJobs = append(pullJobs, BuildJob{
			Label: "Pulling " + img,
			Run:   func(w io.Writer) error { return podman.PullImageTo(img, w) },
		})
	}
	if len(pullJobs) > 0 {
		plan.Fill().Report(os.Stdout)
		RunParallel(pullJobs) //nolint:errcheck
	}

	// Start the services.
	var startJobs []BuildJob
	for _, u := range units {
		unit := u
		label := strings.TrimSuffix(strings.TrimPrefix(unit, "lerd-"), ".timer")
		startJobs = append(startJobs, BuildJob{
			Label: label,
			Run:   func(_ io.Writer) error { return podman.StartUnit(unit) },
		})
	}
	feedback.Header("Starting services")
	RunParallel(startJobs) //nolint:errcheck
	// Same discover_family refresh as runStart: engines and admin UIs come up
	// together here without going through StartService.
	serviceops.RefreshDiscoverFamilyConsumers()

	// Workers exec into the FPM containers and depend on lerd-redis et al.
	// Start them after the service containers are up — same ordering as
	// runStart's phase 1 → phase 2 split. Without this, `lerd install` would
	// leave workers enabled-but-stopped after restoreSiteInfrastructure, since
	// restoreWorker only writes the unit file and defers Start to here.
	workerUnits := append(lifecycle.RegisteredQueueUnits(), lifecycle.RegisteredStripeUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredScheduleUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredReverbUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredFrameworkWorkerUnits()...)
	workerUnits = append(workerUnits, lifecycle.RegisteredTimerUnits()...)
	workerUnits = collapseTimerSiblings(dedupeStrings(workerUnits))
	// Don't resurrect workers the idle engine has gracefully suspended, exactly
	// as runStart does. Without this, `lerd install`/`update` (which re-creates
	// and re-enables every worker via restoreSiteInfrastructure) restarts a
	// deliberately-asleep worker on an idle site and wedges the engine: the
	// registry still records it suspended, so the dashboard shows the site asleep
	// while its workers run and the engine never re-suspends them.
	workerUnits = dropIdleSuspendedUnits(workerUnits)
	if len(workerUnits) == 0 {
		return
	}
	var workerJobs []BuildJob
	for _, u := range workerUnits {
		unit := u
		label := strings.TrimSuffix(strings.TrimPrefix(unit, "lerd-"), ".timer")
		workerJobs = append(workerJobs, BuildJob{
			Label: label,
			Run:   func(_ io.Writer) error { return podman.StartUnit(unit) },
		})
	}
	feedback.Header("Starting workers")
	RunParallel(workerJobs) //nolint:errcheck
}

// launchTray restarts the tray applet, stopping any existing instance first.
// Prefers the systemd service when enabled, otherwise launches the helper
// directly. Start (bootout+bootstrap) rather than Restart (kickstart -k), or
// launchctl hangs waiting for the tray process to die.
func launchTray() error {
	killTray()
	if services.Mgr.IsEnabled("lerd-tray") {
		return services.Mgr.Start("lerd-tray")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(exe, "tray").Start()
}

// trayProcessPatterns match a running tray applet, launched directly or as the
// lerd-tray binary, and nothing else. Anchored at the end because `lerd tray
// off` has to kill the applet from a command line that contains those very
// words, and an unanchored match takes out the command and its shell with it.
var trayProcessPatterns = []string{`lerd tray( --mono)?$`, `lerd-tray$`}

// killTray kills any running lerd tray process.
func killTray() {
	for _, pattern := range trayProcessPatterns {
		exec.Command("pkill", "-f", pattern).Run() //nolint:errcheck
	}
}

// reconcileCustomServices heals custom-service drift on start (issue #678).
// Failures are non-fatal so one bad service can't block the start sequence.
func reconcileCustomServices() {
	res, err := serviceops.ReconcileServices(nil)
	if err != nil {
		feedback.Warn("reconciling services: %v", err)
	}
	// A refreshed definition may add client tools (client_shims), so bring the
	// shim dir in line non-interactively.
	if len(res.DefinitionsRefreshed) > 0 {
		_ = shims.Reconcile(nil)
	}
	for _, name := range res.ConfigsApplied {
		feedback.Warn("applied an updated config to %s and restarted it", name)
	}
	// Default-stack services (mysql, postgres, redis…) don't flow through the custom
	// reconcile above, so apply the same config-drift restart to them: a shipped
	// preset config bump (e.g. a higher max_allowed_packet) must reach a running
	// default service on update, not only on an explicit reinstall.
	for _, name := range config.DefaultPresetNames() {
		if applied, err := serviceops.RestartIfConfigDrifted(name, name); err != nil {
			feedback.Warn("applying config for %s: %v", name, err)
		} else if applied {
			feedback.Warn("applied an updated config to %s and restarted it", name)
		}
	}
	for _, name := range res.QuadletsRegenerated {
		feedback.Warn("regenerated missing unit for %s from its config", name)
	}
	for _, name := range res.OrphansRemoved {
		feedback.Warn("removed orphan service %s (unit with no config; data left intact)", name)
	}
	for _, name := range res.RunningOrphansSkipped {
		feedback.Warn("orphan service %s has no config but its container is running; left as-is (remove with: lerd service remove %s)", name, name)
	}
}

// restoreSiteInfrastructure ensures FPM quadlets, service quadlets, and worker
// units exist for all registered (non-paused) sites. This repairs state after
// an uninstall/reinstall cycle where unit files were deleted but site configs
// (sites.yaml, .lerd.yaml) were preserved.
func restoreSiteInfrastructure() {
	reg, err := config.LoadSites()
	if err != nil {
		return
	}

	seenPHP := map[string]bool{}
	seenSvc := map[string]bool{}
	dirty := false

	// Backfill framework for all sites (including paused) that were linked
	// before detection was added.
	for i, s := range reg.Sites {
		if s.Ignored || s.Framework != "" {
			continue
		}
		if name, ok := config.DetectFrameworkForDir(s.Path); ok {
			reg.Sites[i].Framework = name
			dirty = true
		}
	}

	for _, s := range reg.Sites {
		if s.Paused || s.Ignored {
			continue
		}

		// Restore custom container plist/quadlet for custom container sites.
		// On macOS the plist lives in ~/Library/LaunchAgents; on Linux it is a
		// systemd quadlet. After a reinstall the unit file may be gone even though
		// the site is still registered in sites.yaml and .lerd.yaml is on disk.
		if s.IsCustomContainer() {
			unitName := podman.CustomContainerName(s.Name)
			if !services.Mgr.ContainerUnitInstalled(unitName) {
				proj, _ := config.LoadProjectConfig(s.Path)
				if proj != nil && proj.Container != nil {
					if err := podman.WriteCustomContainerQuadlet(s.Name, s.Path, s.ContainerPort); err != nil {
						feedback.Warn("restoring custom container unit for %s: %v", s.Name, err)
					}
				}
			}
		}

		// Restore the per-site quadlet (and image, if missing) for custom-FPM
		// PHP sites, so they come back up on `lerd start` after a reinstall.
		if s.IsCustomFPM() {
			unitName := podman.CustomFPMContainerName(s.Name)
			if !services.Mgr.ContainerUnitInstalled(unitName) {
				proj, _ := config.LoadProjectConfig(s.Path)
				if proj != nil && proj.Container != nil {
					if !podman.CustomImageExists(s.Name) {
						_ = podman.BuildCustomImage(s.Name, s.Path, proj.Container)
					}
					if err := podman.WriteCustomFPMQuadlet(s.Name, s.PHPVersion); err != nil {
						feedback.Warn("restoring custom FPM unit for %s: %v", s.Name, err)
					}
				}
			}
		}

		// Restore FPM quadlet for this site's PHP version (shared-FPM PHP sites
		// only; custom-FPM sites use their per-site container handled above).
		if !s.IsCustomContainer() && !s.IsHostProxy() && !s.IsCustomFPM() {
			phpVer := s.PHPVersion
			if phpVer == "" {
				cfg, _ := config.LoadGlobal()
				phpVer = cfg.PHP.DefaultVersion
			}
			if phpVer != "" && !seenPHP[phpVer] {
				seenPHP[phpVer] = true
				ensureFPMQuadlet(phpVer) //nolint:errcheck
			}
		}

		// Read .lerd.yaml for service and worker info.
		proj, _ := config.LoadProjectConfig(s.Path)
		if proj == nil {
			continue
		}

		// Restore the host-proxy dev-server worker unit. Phase 2 of runStart
		// launches it (it is enumerated by RegisteredFrameworkWorkerUnits).
		// Bind to the command the user approved at link time: if .lerd.yaml's
		// dev command drifted since (e.g. a git pull), don't silently run the
		// new one, warn and wait for a re-link to re-approve it.
		if s.IsHostProxy() && proj.Proxy != nil {
			if s.HostCommand != "" && proj.Proxy.Command != s.HostCommand {
				feedback.Warn("%s: dev command in .lerd.yaml changed since link; not auto-starting. Run `lerd link` to review and approve.", s.Name)
			} else if w, ok := hostProxyWorker(proj.Proxy); ok && !services.Mgr.IsEnabled(hostProxyWorkerUnit(s.Name)) {
				restoreWorker(s.Name, s.Path, "", hostProxyWorkerName, w)
			}
		}

		// Resolve() returns the rendered CustomService for inline + preset
		// references (e.g. mariadb-11) and (nil, nil) for built-ins. Without
		// it, preset references slipped through to the built-in template path.
		for _, svc := range proj.Services {
			if seenSvc[svc.Name] {
				continue
			}
			seenSvc[svc.Name] = true
			cs, err := svc.Resolve()
			if err != nil {
				feedback.Warn("resolving service %q for %s: %v", svc.Name, s.Name, err)
				continue
			}
			if cs != nil {
				ensureCustomServiceQuadlet(cs) //nolint:errcheck
			} else {
				ensureServiceQuadlet(svc.Name) //nolint:errcheck
			}
		}

		// Restore worker units from saved worker names. The platform helper
		// decides whether to start immediately (Linux) or just write the unit
		// file and let phase 2 of runStart launch it (macOS).
		for _, w := range proj.Workers {
			// Leave a worker the idle engine suspended fully down: don't recreate,
			// enable, or start it. Restoring it here re-enables it (so a later boot
			// resurrects it) and feeds it to the start passes, which is how an idle
			// site ends up with running workers after `lerd install`. The engine
			// owns a suspended worker's lifecycle and resumes it on real activity.
			if containsString(s.IdleSuspendedWorkers, w) {
				continue
			}
			unitName := "lerd-" + w + "-" + s.Name
			parentEnabled := services.Mgr.IsEnabled(unitName)
			phpVersion := s.PHPVersion
			if phpVersion == "" {
				cfg, _ := config.LoadGlobal()
				phpVersion = cfg.PHP.DefaultVersion
			}
			if w == "stripe" {
				if parentEnabled {
					continue
				}
				base := siteURL(s.Path)
				if base != "" {
					StripeRestoreUnit(s.Name, s.Path, base) //nolint:errcheck
				}
				continue
			}
			fwName := s.Framework
			fw, fwOK := config.GetFrameworkForDir(fwName, s.Path)
			if !fwOK || fw.Workers == nil {
				continue
			}
			wDef, ok := fw.Workers[w]
			if !ok {
				continue
			}
			// Skip restore entirely when the platform can't run this worker
			// shape — writeWorkerUnitFile would print a WARN and return
			// (false, nil) for every worktree, every boot.
			if ok, _ := workerSupportedOnPlatform(wDef); !ok {
				continue
			}
			if !parentEnabled {
				restoreWorker(s.Name, s.Path, phpVersion, w, wDef)
			}
			// Per-worktree host workers: rewrite each worktree's unit so
			// stop/start cycles don't leave them stale. The parent unit
			// alone is not enough because PR #319 shipped per-worktree
			// units (lerd-<w>-<site>-<wtBase>) with a separate lifecycle.
			if !wDef.Host {
				continue
			}
			worktrees, err := gitpkg.DetectWorktrees(s.Path, s.PrimaryDomain())
			if err != nil {
				continue
			}
			for _, wt := range worktrees {
				// Leave a worktree worker the idle engine suspended fully down, the
				// same as the main-site guard above: restoreWorker re-enables it (so a
				// later boot's default.target pulls it in, past dropIdleSuspendedUnits)
				// and the engine, not install, owns resuming it on real activity.
				if worktreeWorkerIdleSuspended(&s, wt.Path, w) {
					continue
				}
				if services.Mgr.IsEnabled(WorkerUnitName(s.Name, wt.Path, w)) {
					continue
				}
				wtPHP := config.WorktreePHPVersion(wt.Path, phpVersion)
				restoreWorker(s.Name, wt.Path, wtPHP, w, wDef)
			}
		}
	}
	if dirty {
		config.SaveSites(reg) //nolint:errcheck
	}

	// Restore unit files for standalone custom services (installed globally via
	// `lerd service add`) whose config exists in ~/.config/lerd/services/ but
	// whose unit file (plist on macOS, quadlet on Linux) is missing — e.g. after
	// a reinstall that wiped ~/Library/LaunchAgents or ~/.config/containers/systemd/.
	if customs, err := config.ListCustomServices(); err == nil {
		for _, svc := range customs {
			if !services.Mgr.ContainerUnitInstalled("lerd-" + svc.Name) {
				ensureCustomServiceQuadlet(svc) //nolint:errcheck
			}
		}
	}

	cleanOrphanTimerUnits()

	podman.DaemonReloadFn() //nolint:errcheck
}

// cleanOrphanTimerUnits removes lerd-*.timer files whose sibling .service
// is missing — they can't fire and break parallel start with exit 1.
func cleanOrphanTimerUnits() {
	dir := config.SystemdUserDir()
	entries, _ := filepath.Glob(filepath.Join(dir, "lerd-*.timer"))
	for _, e := range entries {
		base := strings.TrimSuffix(filepath.Base(e), ".timer")
		if _, err := os.Stat(filepath.Join(dir, base+".service")); err == nil {
			continue
		}
		_ = services.Mgr.RemoveTimerUnit(base)
	}
}

// suspendedWorkerUnitSet returns the worker unit names (without any .timer
// suffix) the idle engine currently has suspended across all sites, covering
// both main-site workers (lerd-{worker}-{site}) and per-worktree workers
// (lerd-{worker}-{site}-{wtslug}). Naming matches workerNames.
func suspendedWorkerUnitSet() map[string]bool {
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return nil
	}
	out := map[string]bool{}
	for _, s := range reg.Sites {
		for _, w := range s.IdleSuspendedWorkers {
			out["lerd-"+w+"-"+s.Name] = true
		}
		for wtBase, workers := range s.WorktreeIdleSuspended {
			for _, w := range workers {
				out["lerd-"+w+"-"+s.Name+"-"+wtBase] = true
			}
		}
	}
	return out
}

// dropIdleSuspendedUnits removes idle-suspended worker units from a start list,
// matching on the unit name with any .timer suffix stripped so a suspended
// scheduled worker's timer is dropped too.
func dropIdleSuspendedUnits(units []string) []string {
	return filterSuspendedUnits(units, suspendedWorkerUnitSet())
}

// filterSuspendedUnits is the pure filter behind dropIdleSuspendedUnits: it
// removes any unit whose .timer-stripped name is in suspended.
func filterSuspendedUnits(units []string, suspended map[string]bool) []string {
	if len(suspended) == 0 {
		return units
	}
	out := make([]string, 0, len(units))
	for _, u := range units {
		if suspended[strings.TrimSuffix(u, ".timer")] {
			continue
		}
		out = append(out, u)
	}
	return out
}

// collapseTimerSiblings drops a worker's bare .service entry when its
// .timer sibling is also in the list — the timer is what drives the
// oneshot, the bare .service would just fire schedule:run a second time.
func collapseTimerSiblings(in []string) []string {
	hasTimer := map[string]bool{}
	for _, u := range in {
		if strings.HasSuffix(u, ".timer") {
			hasTimer[strings.TrimSuffix(u, ".timer")] = true
		}
	}
	out := make([]string, 0, len(in))
	for _, u := range in {
		if !strings.HasSuffix(u, ".timer") && hasTimer[u] {
			continue
		}
		out = append(out, u)
	}
	return out
}

// mergeMigrationRestarts unions the containers torn down by the podman-upgrade
// heal with those recreated by a network migration, de-duplicated and order
// preserved, so a run that triggers BOTH restarts every affected container
// exactly once. Overwriting one list with the other left heal-torn-down services
// stopped after install.
func mergeMigrationRestarts(healed, recreated []string) []string {
	return dedupeStrings(append(append([]string(nil), healed...), recreated...))
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// RunStart starts all lerd services (exported for use by the UI server). emit,
// when set, receives one StartEvent per stage and per unit. Pass the caller's
// own unit in skip so the start does not boot the caller out.
func RunStart(emit func(StartEvent), skip ...string) error { return startLerd(emit, skip) }

// dropSkipped removes skip's units from us, leaving the order of the rest.
func dropSkipped(us, skip []string) []string {
	if len(skip) == 0 {
		return us
	}
	return slices.DeleteFunc(us, func(u string) bool { return slices.Contains(skip, u) })
}

// RunStop stops lerd containers (exported for use by the UI server).
func RunStop() error { return runStop(nil, nil) }

// RunQuit stops all lerd processes and containers (exported for use by the UI server).
func RunQuit() error { return runQuit(nil, nil) }

// spinnerRunner runs lifecycle teardown jobs through the CLI's parallel
// spinner UI, so `lerd stop` and `lerd quit` render progress per unit.
func spinnerRunner(jobs []lifecycle.Job) error {
	bjs := make([]BuildJob, len(jobs))
	for i, j := range jobs {
		bjs[i] = BuildJob{Label: j.Label, Run: j.Run}
	}
	return RunParallel(bjs)
}

func runStop(_ *cobra.Command, _ []string) error {
	return lifecycle.Stop(spinnerRunner)
}

func runQuit(_ *cobra.Command, _ []string) error {
	// killTray runs before the VM stop: it clears any directly-launched tray
	// instance launchd and systemd know nothing about, and leaving the icon on
	// screen for the seconds `podman machine stop` takes reads as a hung quit.
	return lifecycle.Quit(spinnerRunner, killTray)
}

// hasControllingTerminal reports whether this run has a terminal behind it.
// /dev/tty rather than stdin is the signal: `lerd start < /dev/null` in a
// terminal still has one, and a launchd job or a Finder-launched app has
// neither. term.IsTerminal on stdin alone gets both wrong.
func hasControllingTerminal() bool {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	tty.Close()
	return true
}

// canPromptForPassword reports whether sudo would have someone to ask: it reads
// the password from the controlling terminal, not from stdin.
func canPromptForPassword() bool { return hasControllingTerminal() }

// dnsEnabled reports whether the user has lerd manage DNS. When off, start must
// not install DNS sudoers grants or touch any resolver state.
func dnsEnabled() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg != nil && cfg.DNS.Enabled
}
