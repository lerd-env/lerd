package sitedoctor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// WorkerEnvGap is a worker the site lists whose check passes except for its
// when_env condition, so the .env value alone keeps it from running.
type WorkerEnvGap struct {
	Worker string
	Key    string
	Want   string
	Got    string
	// Service is the declared service the wanted value points at, read from the
	// framework's env detect rules; empty when the site has none, and only then
	// is the gap left to the user rather than offered as a fix.
	Service string
}

// WorkerEnvGaps lists the env-gated workers of the project at path, in the
// order .lerd.yaml lists them. The check and both fix surfaces share it so the
// fix always writes exactly what the report showed.
func WorkerEnvGaps(path string, fw *config.Framework) []WorkerEnvGap {
	cfg, err := config.LoadProjectConfig(path)
	if err != nil || fw == nil || fw.Workers == nil {
		return nil
	}
	var gaps []WorkerEnvGap
	for _, w := range cfg.Workers {
		def, ok := fw.Workers[w]
		if !ok || def.Check == nil || def.Check.WhenEnv == "" {
			continue
		}
		withoutEnv := *def.Check
		withoutEnv.WhenEnv = ""
		if config.MatchesRule(path, *def.Check) || !config.MatchesRule(path, withoutEnv) {
			continue
		}
		key, want, ok := strings.Cut(def.Check.WhenEnv, "=")
		if !ok {
			continue
		}
		gaps = append(gaps, WorkerEnvGap{
			Worker:  w,
			Key:     key,
			Want:    want,
			Got:     envfile.ReadKey(filepath.Join(path, ".env"), key),
			Service: serviceForEnv(fw, cfg.ServiceNames(), key, want),
		})
	}
	return gaps
}

// serviceForEnv names the declared service whose detect rules claim key=value,
// e.g. QUEUE_CONNECTION=redis is redis's. Only prefixed rules on that key count:
// a bare REDIS_HOST or an absent-key rule says nothing about this value.
func serviceForEnv(fw *config.Framework, declared []string, key, value string) string {
	for _, name := range declared {
		for _, rule := range fw.Env.Services[name].Detect {
			if rule.Key == key && rule.ValuePrefix != "" && !rule.Absent && strings.HasPrefix(value, rule.ValuePrefix) {
				return name
			}
		}
	}
	return ""
}

// checkWorkerEnv reports listed workers that only their .env value keeps from
// running. Absent once nothing is gated, so it never sits there as a green row.
func checkWorkerEnv(path string, fw *config.Framework) (Check, bool) {
	gaps := WorkerEnvGaps(path, fw)
	if len(gaps) == 0 {
		return Check{}, false
	}
	parts := make([]string, 0, len(gaps))
	fixable := false
	for _, g := range gaps {
		got := g.Got
		if got == "" {
			got = "unset"
		}
		parts = append(parts, fmt.Sprintf("%s needs %s=%s (currently %s), lerd skips it", g.Worker, g.Key, g.Want, got))
		fixable = fixable || g.Service != ""
	}
	c := Check{Name: "worker_env", Status: StatusWarn, Detail: strings.Join(parts, "; ")}
	if fixable {
		c.Fix = FixWorkerEnv
	}
	return c, true
}
