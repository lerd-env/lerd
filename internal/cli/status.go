package cli

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/lifecycle"
	"github.com/geodro/lerd/internal/nativephp"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
	"github.com/geodro/lerd/internal/tools"
	lerdUpdate "github.com/geodro/lerd/internal/update"
	"github.com/geodro/lerd/internal/version"
	"github.com/spf13/cobra"
)

func ok2(label string) {
	fmt.Printf("  %s %s\n", feedback.Green(feedback.GlyphOK), label)
}

// paused2 reports an idle-suspended worker: stopped on purpose, resumes on the
// next request, so it's shown green (healthy) as paused rather than missing.
func paused2(label string) {
	fmt.Printf("  %s %s %s\n", feedback.Green(feedback.GlyphOK), label, feedback.Dim("(paused, idle)"))
}

// siteWorkerIdleSuspended reports whether the named worker is currently
// idle-suspended for the site.
func siteWorkerIdleSuspended(s config.Site, worker string) bool {
	for _, w := range s.IdleSuspendedWorkers {
		if w == worker {
			return true
		}
	}
	return false
}

func fail2(label, msg, hint string) {
	fmt.Printf("  %s %s %s\n    %s %s\n", feedback.Red(feedback.GlyphFail), label, feedback.Dim("("+msg+")"), feedback.Dim("hint:"), hint)
}
func warn2(label, msg string) {
	fmt.Printf("  %s %s %s\n", feedback.Amber(feedback.GlyphWarn), label, feedback.Dim("("+msg+")"))
}

// note2 reports something that is deliberately not running, like a PHP version
// no site is pinned to. Neither green nor red: nothing is wrong, and nothing is
// serving either.
func note2(label, msg string) {
	fmt.Printf("  %s %s %s\n", feedback.Dim("·"), label, feedback.Dim("("+msg+")"))
}

// NewStatusCmd returns the status command.
func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show overall Lerd health status",
		RunE:  runStatus,
	}
}

// printNativePHPStatus reports the host pools, which are what serves under the
// native runtime. Looking for images there failed every version at once while
// PHP was up, since none of them has an image and the rebuild it pointed at
// refuses on this runtime.
func printNativePHPStatus(versions []string, running func(string) bool) {
	if len(versions) == 0 {
		warn2("PHP versions", "none installed — run: lerd use 8.4")
		return
	}
	for _, v := range versions {
		if running(v) {
			ok2("PHP " + v)
			continue
		}
		fail2("PHP "+v, "pool not running", "lerd start")
	}
}

// printContainerPHPStatus reports the shared FPM container per version, which
// needs both an image to run and the container up to serve.
// phpRowState is how a PHP version's status row reads. A version no site is
// pinned to is idle rather than broken: lerd never starts it, and `lerd fetch`
// never builds it, so failing the row asks the reader to repair a deliberate
// absence.
type phpRowState int

const (
	phpRowOK phpRowState = iota
	phpRowImageMissing
	phpRowDown
	phpRowNotBuilt
	phpRowIdle
)

func phpVersionRowState(imageExists, running, used bool) phpRowState {
	if !imageExists {
		if used {
			return phpRowImageMissing
		}
		return phpRowNotBuilt
	}
	if running {
		return phpRowOK
	}
	if used {
		return phpRowDown
	}
	return phpRowIdle
}

func printContainerPHPStatus() {
	versions, _ := phpPkg.ListInstalled()
	if len(versions) == 0 {
		warn2("PHP versions", "none installed — run: lerd use 8.4")
		return
	}
	used := map[string]bool{}
	for _, v := range versionsInUse(versions) {
		used[v] = true
	}
	for _, v := range versions {
		unit := "lerd-php" + strings.ReplaceAll(v, ".", "") + "-fpm"
		imageExists := podman.RunSilent("image", "exists", unit+":local") == nil
		running := false
		if imageExists {
			running, _ = podman.ContainerRunning(unit)
		}
		label := "PHP " + v + " FPM"
		switch phpVersionRowState(imageExists, running, used[v]) {
		case phpRowOK:
			ok2(label)
		case phpRowImageMissing:
			fail2(label, "image missing", "lerd php:rebuild "+v)
		case phpRowDown:
			fail2(label, unit+" not running", serviceStartHint(unit))
		case phpRowNotBuilt:
			note2(label, "not built — no site uses it; build with: lerd php:rebuild "+v)
		case phpRowIdle:
			note2(label, "idle — no site uses it")
		}
	}
}

// runtimeSwitchBanner warns that the rows below are mid-switch. Containers stop
// and start and sites answer 500 for a few seconds, so a red row there is the
// switch in progress rather than something to repair.
func runtimeSwitchBanner(switching bool) string {
	if !switching {
		return ""
	}
	return "\n  ⟳ a PHP runtime switch is running; anything down below is mid-move. Run this again once it finishes."
}

func runStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	fmt.Println("Lerd Status")
	fmt.Println("═══════════════════════════════════════")
	if banner := runtimeSwitchBanner(config.RuntimeSwitchInProgress()); banner != "" {
		fmt.Println(banner)
	}

	// DNS check
	fmt.Println("\n[DNS]")
	tld := dns.ConfiguredTLD()
	if !cfg.DNS.Enabled {
		ok2(fmt.Sprintf("DNS managed externally (.%s)", tld))
	} else {
		switch dns.CheckStatus(tld) {
		case dns.StatusOK:
			ok2(fmt.Sprintf(".%s resolution", tld))
		case dns.StatusDegraded:
			warn2(fmt.Sprintf(".%s resolution", tld),
				"lerd-dns healthy, system resolver bypassed (VPN?)")
		default:
			fail2(fmt.Sprintf(".%s resolution", tld),
				"not resolving",
				dnsRestartHint())
		}
	}

	// Nginx
	fmt.Println("\n[Nginx]")
	running, _ := podman.ContainerRunning("lerd-nginx")
	if running {
		ok2("lerd-nginx container")
	} else {
		fail2("lerd-nginx container",
			"not running",
			serviceStatusHint("lerd-nginx"))
	}

	// PHP
	if cfg.PHPRuntimeMode() == config.PHPRuntimeNative {
		fmt.Println("\n[PHP (native)]")
		printNativePHPStatus(nativephp.ListInstalled(), nativephp.Running)
	} else {
		fmt.Println("\n[PHP FPM]")
		printContainerPHPStatus()
	}

	// Custom Containers
	if customUnits := lifecycle.InstalledCustomContainerUnits(); len(customUnits) > 0 {
		fmt.Println("\n[Custom Containers]")
		for _, unit := range customUnits {
			running, _ := podman.ContainerRunning(unit)
			if running {
				ok2(unit)
			} else {
				fail2(unit, "not running", serviceStartHint(unit))
			}
		}
	}

	// Watcher
	fmt.Println("\n[Watcher]")
	if services.Mgr.IsActive("lerd-watcher") {
		ok2("lerd-watcher")
	} else {
		fail2("lerd-watcher", "not running", serviceStartHint("lerd-watcher"))
	}

	// Tools — the host binaries lerd manages, against their pinned versions.
	fmt.Println("\n[Tools]")
	for _, s := range tools.StatusAll(context.Background()) {
		if s.Name == "fnm" && cfg.NodeManager() == "nvm" {
			continue // deliberately absent on nvm-managed setups
		}
		switch {
		case !s.Present:
			warn2(s.Name, "not installed — run: lerd install")
		case s.UpdateAvailable:
			warn2(s.Name+" "+s.Installed, s.Pinned+" available — run: lerd tools:update")
		case s.Installed == "":
			warn2(s.Name, "version unknown — refresh with: lerd tools:update")
		default:
			ok2(s.Name + " " + s.Installed)
		}
	}

	// Services — only show services that have a quadlet file installed
	fmt.Println("\n[Services]")
	installedCount := 0
	for _, svc := range knownServices() {
		unit := "lerd-" + svc
		if !services.Mgr.ContainerUnitInstalled(unit) {
			continue
		}
		installedCount++
		status, _ := services.Mgr.UnitStatus(unit)
		label := svc
		if ver := podman.ServiceVersionLabel(podman.InstalledImage(unit)); ver != "" {
			label = svc + " " + ver
		}
		switch status {
		case "active":
			ok2(label)
		case "inactive":
			if config.CountSitesUsingService(svc) == 0 {
				warn2(label, "no sites using this service")
			} else {
				warn2(label, "inactive — start with: lerd service start "+svc)
			}
		default:
			fail2(label, status, serviceStatusHint(unit))
		}
	}
	customs, _ := config.ListCustomServices()
	for _, svc := range customs {
		unit := "lerd-" + svc.Name
		if !services.Mgr.ContainerUnitInstalled(unit) {
			continue
		}
		installedCount++
		status, _ := services.Mgr.UnitStatus(unit)
		tag := "[custom]"
		if svc.Preset != "" {
			tag = "[preset]"
		}
		label := svc.Name
		if ver := podman.ServiceVersionLabel(svc.Image); ver != "" {
			label = svc.Name + " " + ver
		}
		label = label + " " + tag
		switch status {
		case "active":
			ok2(label)
		case "inactive":
			if config.CountSitesUsingService(svc.Name) == 0 {
				warn2(label, "no sites using this service")
			} else {
				warn2(label, "inactive — start with: lerd service start "+svc.Name)
			}
		default:
			fail2(label, status, serviceStatusHint(unit))
		}
	}
	if installedCount == 0 {
		fmt.Println("  No services installed. Start one with: lerd service start <name>")
	}

	// Workers
	fmt.Println("\n[Workers]")
	{
		workerReg, wErr := config.LoadSites()
		if wErr == nil {
			hasWorkers := false
			failedCount := 0
			for _, s := range workerReg.Sites {
				if s.Ignored || s.Paused {
					continue
				}
				// Check built-in worker types.
				for _, w := range []string{"queue", "schedule", "reverb", "horizon"} {
					unit := "lerd-" + w + "-" + s.Name
					if siteWorkerIdleSuspended(s, w) {
						paused2(fmt.Sprintf("%s/%s", s.Name, w))
						hasWorkers = true
						continue
					}
					status, _ := podman.UnitStatus(unit)
					switch status {
					case "active":
						ok2(fmt.Sprintf("%s/%s", s.Name, w))
						hasWorkers = true
					case "activating":
						warn2(s.Name+"/"+w, "restarting — check logs: "+unitLogHint(unit))
						hasWorkers = true
					case "failed":
						fail2(s.Name+"/"+w, "failed", unitLogHint(unit))
						hasWorkers = true
						failedCount++
					}
				}
				// Check custom framework workers.
				fwName := s.Framework
				if fw, ok := config.GetFrameworkForDir(fwName, s.Path); ok && fw.Workers != nil {
					for wName, wDef := range fw.Workers {
						switch wName {
						case "queue", "schedule", "reverb", "horizon":
							continue
						}
						if wDef.Check != nil && !config.MatchesRule(s.Path, *wDef.Check) {
							continue
						}
						unit := "lerd-" + wName + "-" + s.Name
						if siteWorkerIdleSuspended(s, wName) {
							paused2(fmt.Sprintf("%s/%s", s.Name, wName))
							hasWorkers = true
							continue
						}
						status, _ := podman.UnitStatus(unit)
						switch status {
						case "active":
							ok2(fmt.Sprintf("%s/%s", s.Name, wName))
							hasWorkers = true
						case "activating":
							warn2(s.Name+"/"+wName, "restarting — check logs: "+unitLogHint(unit))
							hasWorkers = true
						case "failed":
							fail2(s.Name+"/"+wName, "failed", unitLogHint(unit))
							hasWorkers = true
							failedCount++
						}
					}
				}
				// Stripe listener.
				if siteWorkerIdleSuspended(s, "stripe") {
					paused2(fmt.Sprintf("%s/stripe", s.Name))
					hasWorkers = true
				} else if stripeStatus, _ := podman.UnitStatus("lerd-stripe-" + s.Name); stripeStatus == "active" {
					ok2(fmt.Sprintf("%s/stripe", s.Name))
					hasWorkers = true
				} else if stripeStatus == "failed" || stripeStatus == "activating" {
					label := s.Name + "/stripe"
					if stripeStatus == "activating" {
						warn2(label, "restarting")
					} else {
						fail2(label, "failed", unitLogHint("lerd-stripe-"+s.Name))
						failedCount++
					}
					hasWorkers = true
				}
			}
			if !hasWorkers {
				fmt.Println("  No workers running.")
			}
			// One consolidated heal hint per status run. Per-line hints
			// already point at journalctl for the underlying cause; this
			// surfaces the recovery primitive once instead of N times.
			if failedCount > 0 {
				fmt.Printf("\n  %d failed worker(s). Reset and restart with: %s\n",
					failedCount, feedback.Amber("lerd worker heal"))
			}
		}
	}

	// Certificate expiry for secured sites
	fmt.Println("\n[TLS Certificates]")
	reg, err := config.LoadSites()
	if err == nil {
		hasSecured := false
		for _, s := range reg.Sites {
			if s.Ignored || !s.Secured {
				continue
			}
			hasSecured = true
			certPath := filepath.Join(config.CertsDir(), "sites", s.PrimaryDomain()+".crt")
			if exp, err := certExpiry(certPath); err != nil {
				fail2(s.PrimaryDomain(), "cannot read cert", "run: lerd secure "+s.PrimaryDomain())
			} else {
				remaining := time.Until(exp)
				days := int(remaining.Hours() / 24)
				if days < 30 {
					warn2(s.PrimaryDomain(), fmt.Sprintf("expires in %d days", days))
				} else {
					ok2(fmt.Sprintf("%s (expires in %d days)", s.PrimaryDomain(), days))
				}
			}
		}
		if !hasSecured {
			fmt.Println("  No secured sites.")
		}
	}

	// LAN exposure + remote dashboard access
	fmt.Println("\n[Remote Access]")
	lanIP, _ := detectPrimaryLANIP()
	printRemoteAccessStatus(cfg, lanIP)

	// Update notice
	if info, _ := lerdUpdate.CachedUpdateCheck(version.Version); info != nil {
		printUpdateNotice(info)
	}

	fmt.Println()
	return nil
}

// printRemoteAccessStatus renders the [Remote Access] section of `lerd status`.
// Split out from runStatus so it can be tested without mocking podman/DNS/sites.
// lanIP may be empty — the caller is responsible for detection so tests can
// inject a deterministic value.
func printRemoteAccessStatus(cfg *config.GlobalConfig, lanIP string) {
	if cfg.LAN.Exposed {
		ip := lanIP
		if ip == "" {
			ip = "(unknown)"
		}
		ok2(fmt.Sprintf("LAN exposure (%s)", ip))
	} else {
		warn2("LAN exposure", "loopback only — enable with: lerd lan expose")
	}
	switch {
	case cfg.LAN.ServicesExposed && cfg.LAN.Exposed:
		ok2("Managed service LAN access")
	case cfg.LAN.ServicesExposed:
		warn2("Managed service LAN access", "enabled but inactive until LAN exposure is on")
	default:
		ok2("Managed service LAN access (off; services loopback-only)")
	}
	if cfg.UI.PasswordHash != "" {
		ok2(fmt.Sprintf("Dashboard remote access (user: %s)", cfg.UI.Username))
	} else {
		warn2("Dashboard remote access", "LAN clients get 403 — enable with: lerd remote-control on")
	}
}

// printUpdateNotice prints a highlighted banner when a new lerd version is available.
func printUpdateNotice(info *lerdUpdate.UpdateInfo) {
	bar := "══════════════════════════════════════════════"
	fmt.Println()
	fmt.Println(feedback.Amber(bar))
	fmt.Println(feedback.Amber("  Update available: " + info.LatestVersion + "  →  run: lerd update"))
	fmt.Println(feedback.Amber("  Run lerd whatsnew to see what changed."))
	fmt.Println(feedback.Amber(bar))
}

// certExpiry reads the expiry date from a PEM certificate file.
func certExpiry(path string) (time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, fmt.Errorf("no PEM block found")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.NotAfter, nil
}
