package cli

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// runSiteEnv runs `lerd env` in a site's folder. A seam for the tests.
var runSiteEnv = func(dir string) ([]byte, error) { return envCommandFor(dir).CombinedOutput() }

// ensurePreset fetches a preset definition from the store. A seam for the tests.
var ensurePreset = config.EnsurePreset

// presetsFetched remembers the preset names already asked of the store.
var presetsFetched = map[string]bool{}

// pulledServicesTried remembers, per site, the unwired set last acted on and
// when. env rewrites .env, which wakes the watcher again, so a set that failed
// (a pull that timed out) is retried only once pulledRetryAfter has passed.
var (
	pulledServicesMu    sync.Mutex
	pulledServicesTried = map[string]pulledAttempt{}
)

type pulledAttempt struct {
	key       string
	at        time.Time
	envFailed []string // services whose env run failed, retried after the back-off
}

// pulledNow is the clock the back-off reads. A seam for the tests.
var pulledNow = time.Now

const pulledRetryAfter = 5 * time.Minute

// unwiredProjectServices are the services .lerd.yaml declares that the env does
// not point at (the doctor's finding) and lerd never wired, such as one a pull
// brought in. One lerd wired before was repointed by hand and is left alone.
func unwiredProjectServices(site *config.Site, proj *config.ProjectConfig, fw *config.Framework) []config.ProjectService {
	if proj == nil {
		return nil
	}
	unwired := sitedoctor.UnwiredServices(site.Path, fw)
	var out []config.ProjectService
	for _, svc := range proj.Services {
		if !slices.Contains(unwired, svc.Name) || slices.Contains(site.WiredServices, svc.Name) ||
			site.DeclinesService(svc.Name) || config.ServiceIsRemoved(svc.Name) {
			continue
		}
		out = append(out, svc)
	}
	return out
}

// WirePulledServices starts the services a change to .lerd.yaml added and
// rewrites the site's env to point at them, returning the ones it applied.
// A new inline container is left for `lerd link`, which shows it and asks.
func WirePulledServices(site *config.Site) []string {
	// Pausing stops a site's services on purpose; resume brings them back.
	if site.Paused {
		return nil
	}
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok {
		return nil
	}
	if names, due := retryFailedEnv(site); due {
		return names
	}
	fetchMissingPresets(proj)
	unwired := unwiredProjectServices(site, proj, fw)
	key := strings.Join(serviceNamesOf(unwired), ",")
	if key == "" {
		return nil
	}
	now := pulledNow()
	pulledServicesMu.Lock()
	last := pulledServicesTried[site.Path]
	tried := last.key == key && now.Sub(last.at) < pulledRetryAfter
	if !tried {
		pulledServicesTried[site.Path] = pulledAttempt{key: key, at: now}
	}
	pulledServicesMu.Unlock()
	if tried {
		return nil
	}

	apply := &config.ProjectConfig{}
	for _, svc := range unwired {
		if _, err := config.LoadCustomService(svc.Name); err == nil {
			// Installed already: start the one on disk, never replace it from the repo.
			apply.Services = append(apply.Services, config.ProjectService{Name: svc.Name})
			continue
		}
		if svc.Custom != nil {
			fmt.Printf("[WARN] %s declares a new service %s; run `lerd link` there to review and install it\n", site.Name, svc.Name)
			continue
		}
		apply.Services = append(apply.Services, svc)
	}
	if len(apply.Services) == 0 {
		return nil
	}
	if err := linkApplyServices(site.Path, *site, apply); err != nil {
		fmt.Printf("[WARN] starting new services for %s: %v\n", site.Name, err)
	}
	// env writes the keys even for a container that never came up, which would
	// mark it wired; hold it so the back-off retries the install instead.
	for _, svc := range apply.Services {
		if err := ensureServiceRunning(svc.Name); err != nil {
			fmt.Printf("[WARN] %s is not running for %s, will retry: %v\n", svc.Name, site.Name, err)
			return nil
		}
	}
	return runPulledEnv(site, serviceNamesOf(apply.Services))
}

// retryFailedEnv reruns env for a site whose last run failed once the back-off
// has passed. env marks a service wired before it learns its database or bucket
// could not be made, so nothing else would bring the site back here.
func retryFailedEnv(site *config.Site) ([]string, bool) {
	pulledServicesMu.Lock()
	last := pulledServicesTried[site.Path]
	pulledServicesMu.Unlock()
	if len(last.envFailed) == 0 || pulledNow().Sub(last.at) < pulledRetryAfter {
		return nil, false
	}
	return runPulledEnv(site, last.envFailed), true
}

// runPulledEnv runs env for the site and records whether it has to be retried,
// returning names when it succeeded.
func runPulledEnv(site *config.Site, names []string) []string {
	out, err := runSiteEnv(site.Path)
	pulledServicesMu.Lock()
	a := pulledServicesTried[site.Path]
	a.at, a.envFailed = pulledNow(), nil
	if err != nil {
		a.envFailed = names
	}
	pulledServicesTried[site.Path] = a
	pulledServicesMu.Unlock()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		fmt.Printf("[WARN] lerd env for %s, will retry: %v: %s\n", site.Name, err, lines[len(lines)-1])
		return nil
	}
	return names
}

// fetchMissingPresets caches the store definition of a declared preset this
// machine has never seen, such as valkey, since only it says which role the
// service fills. The doctor reads presets offline and would miss it otherwise.
func fetchMissingPresets(proj *config.ProjectConfig) {
	for _, svc := range proj.Services {
		name := svc.Preset
		if name == "" {
			name = svc.Name
		}
		if svc.Custom != nil || name == "sqlite" || config.PresetExists(name) {
			continue
		}
		if _, err := config.LoadCustomService(svc.Name); err == nil {
			continue
		}
		pulledServicesMu.Lock()
		asked := presetsFetched[name]
		presetsFetched[name] = true
		pulledServicesMu.Unlock()
		if !asked {
			_, _ = ensurePreset(name)
		}
	}
}

func serviceNamesOf(svcs []config.ProjectService) []string {
	out := make([]string, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, s.Name)
	}
	return out
}
