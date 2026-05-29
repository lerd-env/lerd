package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// tuningMount describes where a service family's user tuning override is
// bind-mounted inside the container, plus the commented template lerd seeds on
// first use. Only families listed in tuningMounts expose a tuning file as a
// FALLBACK; user-authored services can also declare their own tuning via the
// inline TuningSpec on CustomService, which takes precedence over the family
// map.
type tuningMount struct {
	// Target is the absolute in-container path the override is mounted to. It
	// is chosen so the container's config loader reads it after the bundled
	// preset config, letting user values win.
	Target string
	// Template is the seed body written when the host file does not yet exist.
	Template string
	// Command, when set, is the container command that makes the image read the
	// override. It is only applied when the service has no Exec of its own (the
	// bundled presets don't), so families whose image auto-includes a conf
	// directory (mysql/mariadb) leave this empty. e.g. redis must be told
	// "redis-server <conf>" since it loads no config file by default.
	Command string
}

// TuningSpec is the YAML shape user-authored CustomService entries use to
// declare their own tuning override without having to match a recognised
// family. Set svc.Tuning to expose the Config tab + lerd service config
// for any image whose config loader reads from a specific in-container
// path.
//
//	tuning:
//	  target: /etc/memcached.conf
//	  template: |
//	    # Lerd user tuning for this memcached service.
//	    # -m 128
//	  command: memcached -f /etc/memcached.conf   # optional, only when the
//	                                                # image needs to be told
//	                                                # to read the file
type TuningSpec struct {
	// Target is the absolute in-container path the override is mounted
	// to. Required. Lerd bind-mounts the host file
	// (ServiceTuningFile(svc.Name)) read-only at this path; the image
	// must auto-include the file or be pointed at it via Command.
	Target string `yaml:"target"`
	// Template is the body lerd seeds the host file with on first use.
	// Optional; defaults to an empty file. Conventionally a commented
	// hint sheet so users discover the available knobs without lerd
	// having to ship working defaults.
	Template string `yaml:"template,omitempty"`
	// Command, when set, overrides the container Exec so the image
	// actually loads the override (e.g. `redis-server <path>` for an
	// image that loads no config by default). Leave empty for images
	// whose entrypoint already auto-includes the target path.
	Command string `yaml:"command,omitempty"`
}

// inlineMount converts a YAML-declared TuningSpec into the same internal
// tuningMount shape the family-keyed map uses, so every lookup downstream
// can stay polymorphic over "inline OR family".
func (s *TuningSpec) inlineMount() (tuningMount, bool) {
	if s == nil || s.Target == "" {
		return tuningMount{}, false
	}
	return tuningMount{Target: s.Target, Template: s.Template, Command: s.Command}, true
}

// resolveTuningMount returns the effective tuningMount for svc, preferring
// the inline YAML TuningSpec when present so a user-authored custom service
// can opt into the Config tab without lerd having to recognise its family.
// Falls back to the family-keyed tuningMounts map for the built-in mysql /
// mariadb / redis presets.
func resolveTuningMount(svc *CustomService) (tuningMount, bool) {
	if svc == nil {
		return tuningMount{}, false
	}
	if m, ok := svc.Tuning.inlineMount(); ok {
		return m, true
	}
	m, ok := tuningMounts[FamilyOf(svc)]
	return m, ok
}

// mysqlTuningTemplate seeds the mysql/mariadb override. The zz- filename prefix
// makes it sort after the bundled /etc/mysql/conf.d/lerd.cnf, so anything the
// user sets here overrides the defaults. Everything ships commented out so the
// file is an inert no-op until the user opts in.
const mysqlTuningTemplate = `[mysqld]
# Lerd user tuning for this service.
#
# Lerd created this file once and will never overwrite it, so your edits survive
# ` + "`lerd service reinstall`" + ` and ` + "`lerd update`" + `. It loads after the bundled
# config, so any value set here wins. Uncomment, tune, then run
# ` + "`lerd service restart <name>`" + ` to apply.

# max_allowed_packet = 1G
# innodb_buffer_pool_size = 512M
# innodb_log_file_size = 256M
# max_connections = 200
`

// redisTuningTemplate seeds the redis override. Redis loads no config file by
// default, so the override is passed to redis-server as its config (see the
// Command below). Leaving "dir" unset keeps redis writing to its WORKDIR (/data,
// the mounted data dir), so persistence is unaffected.
const redisTuningTemplate = `# Lerd user tuning for this service.
#
# Lerd created this file once and will never overwrite it, so your edits survive
# ` + "`lerd service reinstall`" + ` and ` + "`lerd update`" + `. redis-server loads it on
# startup. Uncomment, tune, then run ` + "`lerd service restart redis`" + ` to apply.

# maxmemory 256mb
# maxmemory-policy allkeys-lru
# appendonly no
# save ""
`

// tuningMounts maps a service family to its tuning mount. mysql and mariadb
// are distinct families (see their presets) but share the same conf.d include
// path. redis needs a Command because its image loads no config by default.
//
// Note: postgres is intentionally NOT here. The natural shape — pointing at an
// external conf.d via `postgres -c include_dir=...` — does not work, because
// include_dir is a postgresql.conf directive parsed during config-file load,
// before -c runtime parameters are applied. Postgres tuning needs an
// entrypoint-wrapper approach that appends the include line to postgresql.conf
// before postgres starts, which is a separate PR with proper image-version
// runtime verification (lerd's default postgis image rejected the -c form
// outright with FATAL unrecognized configuration parameter "include_dir").
var tuningMounts = map[string]tuningMount{
	"mysql": {
		Target:   "/etc/mysql/conf.d/zz-lerd-user.cnf",
		Template: mysqlTuningTemplate,
	},
	"mariadb": {
		Target:   "/etc/mysql/conf.d/zz-lerd-user.cnf",
		Template: mysqlTuningTemplate,
	},
	"redis": {
		Target:   "/etc/redis/lerd-user.conf",
		Template: redisTuningTemplate,
		Command:  "redis-server /etc/redis/lerd-user.conf",
	},
}

// ResolveServiceForTuning loads the service definition behind name for tuning
// purposes, whether it is a user custom service (a YAML in the services dir) or
// a built-in default preset (e.g. the default mysql, which has no YAML on disk).
// Both kinds render their quadlet through EnsureCustomServiceQuadlet, so the
// resolved value carries the Family that ServiceTuningMount keys off.
func ResolveServiceForTuning(name string) (*CustomService, error) {
	if svc, err := LoadCustomService(name); err == nil {
		return svc, nil
	}
	if IsDefaultPreset(name) {
		p, err := LoadPreset(name)
		if err != nil {
			return nil, err
		}
		return p.Resolve("")
	}
	return nil, fmt.Errorf("service %q is not installed", name)
}

// TuningFamilies returns the sorted list of service families that expose a
// tuning override. Callers use it to render an honest "supported: …" hint in
// error messages, so it stays in sync as new families are added to
// tuningMounts.
func TuningFamilies() []string {
	families := make([]string, 0, len(tuningMounts))
	for f := range tuningMounts {
		families = append(families, f)
	}
	sort.Strings(families)
	return families
}

// ServiceTuningMount returns the in-container mount target for svc's tuning
// override and whether the service exposes one. The matching host file is
// ServiceTuningFile(svc.Name). A service exposes a mount when either it
// declares one inline via svc.Tuning, or its family is in the built-in
// tuningMounts map (mysql / mariadb / redis). Returns ok=false otherwise.
func ServiceTuningMount(svc *CustomService) (target string, ok bool) {
	m, ok := resolveTuningMount(svc)
	if !ok {
		return "", false
	}
	return m.Target, true
}

// ServiceTuningCommand returns the container command that makes svc's image
// read its tuning override, and whether one applies. ok is false unless the
// effective tuningMount declares a Command (mysql/mariadb auto-include their
// conf dir and need none; redis sets one because the image loads no config
// file by default). Callers should only use it when the service has no Exec
// of its own.
func ServiceTuningCommand(svc *CustomService) (command string, ok bool) {
	m, found := resolveTuningMount(svc)
	if !found || m.Command == "" {
		return "", false
	}
	return m.Command, true
}

// MaterializeServiceTuning seeds svc's tuning override with its commented
// template when the host file does not exist yet, and is a no-op once the file
// is present so user edits are never clobbered. Services without a tuning
// mount (neither inline nor family-keyed) are skipped. Call this before
// GenerateCustomQuadlet so the mounted host path always exists.
func MaterializeServiceTuning(svc *CustomService) error {
	m, ok := resolveTuningMount(svc)
	if !ok {
		return nil
	}
	path := ServiceTuningFile(svc.Name)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(m.Template), 0644)
}

// ServiceTuningTemplate returns the commented template for svc's tuning
// mount so callers (most notably the Reset endpoint) can restore the file
// to the "no active directives" state without deleting it. Deleting the
// file is unsafe in practice because the generated quadlet declares a
// Volume= bind mount at the same path; a missing source path makes
// podman refuse to start the container. Overwriting with the template
// keeps the mount valid while making the service fall back to its
// bundled defaults. Inline TuningSpec entries with no Template field
// return ok=true with the empty string, which is still a valid "reset"
// target.
func ServiceTuningTemplate(svc *CustomService) (string, bool) {
	m, ok := resolveTuningMount(svc)
	if !ok {
		return "", false
	}
	return m.Template, true
}
