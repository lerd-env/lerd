package config

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ServiceConfig holds configuration for an optional service.
type ServiceConfig struct {
	Enabled    bool     `yaml:"enabled"        mapstructure:"enabled"`
	Image      string   `yaml:"image"          mapstructure:"image"`
	Port       int      `yaml:"port"           mapstructure:"port"`
	ExtraPorts []string `yaml:"extra_ports"    mapstructure:"extra_ports"`
	// PublishedPort overrides the host (published) port of this service's
	// primary mapping. 0 = use the preset/version default (e.g. 3306 for MySQL
	// 8.4). Set it to free the default port for a second server — e.g. move
	// lerd-mysql to 3307 so the host system MySQL can keep 127.0.0.1:3306. The
	// container-internal port is unchanged, so bridge clients still use 3306.
	// Unlike Port (auto-seeded from the preset), this stays 0 until the user
	// explicitly overrides, so it is an unambiguous "use default" sentinel.
	PublishedPort int `yaml:"published_port,omitempty" mapstructure:"published_port"`
	// PublishedPorts overrides the host (published) side of this service's
	// secondary mappings, keyed by container-internal port. A multi-port service
	// (mailpit's 8025 UI behind the 1025 SMTP primary, rustfs' 9001 console)
	// exposes ports past the primary; this lets the user remap each one while the
	// primary stays in PublishedPort. Empty = every secondary on its preset
	// default. Keyed on the container port because that is the stable identity
	// when the host side moves.
	PublishedPorts map[int]int `yaml:"published_ports,omitempty" mapstructure:"published_ports"`
	PreviousImage  string      `yaml:"previous_image,omitempty" mapstructure:"previous_image"`
	// LastOp records the most recent mutation kind ("update" or "migrate") so
	// the rollback flow can refuse a swap that would race the new image
	// against the post-migrate (fresh) data dir. Empty means no recent op or a
	// state predating the field — treated as plain update for compatibility.
	LastOp string `yaml:"last_op,omitempty" mapstructure:"last_op"`
	// PreMigrateBackup is the absolute path to the data dir that was preserved
	// when the most recent operation was a migrate. Used by rollback to refuse
	// (or, in future, restore) when undoing the migrate would corrupt data.
	PreMigrateBackup string `yaml:"pre_migrate_backup,omitempty" mapstructure:"pre_migrate_backup"`
	// CanonicalVersion pins the preset version tag this service was first
	// installed on, so flipping the YAML's canonical (e.g. pg 16 → 18 in a
	// future release) never silently major-jumps existing installs.
	CanonicalVersion string `yaml:"canonical_version,omitempty" mapstructure:"canonical_version"`
}

// GlobalConfig is the top-level lerd configuration.
type GlobalConfig struct {
	// Editor is the command lerd runs to open a file at a line (the
	// "open in editor" links in the dashboard). Optional {file} and {line}
	// placeholders; if omitted, lerd appends the file. Empty = autodetect a
	// known GUI editor (code/cursor/phpstorm/subl/zed), then xdg-open/open.
	Editor string `yaml:"editor,omitempty" mapstructure:"editor"`
	PHP    struct {
		DefaultVersion string            `yaml:"default_version" mapstructure:"default_version"`
		XdebugEnabled  map[string]bool   `yaml:"xdebug_enabled"  mapstructure:"xdebug_enabled"`
		XdebugMode     map[string]string `yaml:"xdebug_mode,omitempty" mapstructure:"xdebug_mode"`
		// XdebugStart maps a PHP version to its xdebug.start_with_request value
		// (yes | trigger | no). Absent means the default "yes" (connect on every
		// request). "trigger"/"no" support on-demand debugging via the control
		// socket without flooding the IDE from every request and worker.
		XdebugStart map[string]string `yaml:"xdebug_start,omitempty" mapstructure:"xdebug_start"`
		// Extensions is the custom extension set (lerd php:ext), applied to every
		// PHP image lerd builds. Extensions belong to the user, not to a version:
		// keying them per version made a site lose them on a version switch.
		Extensions []string `yaml:"extensions"      mapstructure:"extensions"`
		// ExtApkDeps maps a custom extension name to extra Alpine packages its
		// build needs. Keyed by extension (deps don't vary by PHP version).
		// lerd already knows the deps for some extensions; this is for the rest.
		ExtApkDeps map[string][]string `yaml:"ext_apk_deps,omitempty" mapstructure:"ext_apk_deps"`
		// Packages is the extra Alpine package set (lerd php:pkg) installed in
		// every FPM image's runtime stage, for CLI tools and runtime libraries
		// users want in the container; re-applied on rebuild.
		Packages []string `yaml:"packages,omitempty" mapstructure:"packages"`
		// Realised records what each version's image actually loaded, verified
		// after its build. The declared set above is what the user asked for; not
		// every version can honour all of it (mongodb needs 8.1+, the 7.4/8.0
		// images are Alpine 3.16), and lerd must never advertise what an image
		// does not have.
		Realised map[string]RealisedPHPSet `yaml:"realised,omitempty" mapstructure:"realised"`
		// FPMPorts maps a PHP version to extra host ports published on that
		// version's shared FPM container, so a process bound inside `lerd shell`
		// (a Vite dev server, a websocket, an ad-hoc listener) is reachable at
		// localhost:PORT. Environment-wide per version, not per site; the one
		// shared FPM container per version owns the list, so two sites wanting
		// the same in-container port on the same version collide. Each mapping is
		// a "host:container" spec; the loopback/LAN bind is applied centrally on
		// write. Managed via the PHP page's Ports tab (lerd php:ports).
		FPMPorts map[string][]string `yaml:"fpm_ports,omitempty" mapstructure:"fpm_ports"`
	} `yaml:"php" mapstructure:"php"`
	Node struct {
		DefaultVersion string `yaml:"default_version" mapstructure:"default_version"`
		// Managed records whether lerd manages Node.js via version-manager
		// shims. A pointer so a config predating the field (nil) keeps the
		// historical shim-presence behaviour, while an explicit false survives
		// updates that would otherwise re-add the shims a `node:unmanage` removed.
		Managed *bool `yaml:"managed,omitempty" mapstructure:"managed"`
		// Manager selects the Node version manager lerd drives: "fnm" (the
		// bundled default) or "nvm" (a user-installed nvm). Empty means fnm so
		// configs predating the field keep working unchanged.
		Manager string `yaml:"manager,omitempty" mapstructure:"manager"`
		// NvmDir is the nvm install directory when Manager is "nvm". Persisted
		// at install/switch time so daemons (lerd-ui, watcher) find nvm even
		// though systemd/launchd never load the user's shell rc that exports
		// $NVM_DIR. Empty means fall back to $NVM_DIR or ~/.nvm.
		NvmDir string `yaml:"nvm_dir,omitempty" mapstructure:"nvm_dir"`
	} `yaml:"node" mapstructure:"node"`
	Share struct {
		// DefaultTool is the tunnel tool "lerd share" uses when no flag is
		// given: ngrok | cloudflare | expose | serveo | localhost-run.
		// Empty = auto-detect. Set via "lerd share:tool".
		DefaultTool string `yaml:"default_tool,omitempty" mapstructure:"default_tool"`
	} `yaml:"share,omitempty" mapstructure:"share"`
	Nginx struct {
		HTTPPort  int `yaml:"http_port"  mapstructure:"http_port"`
		HTTPSPort int `yaml:"https_port" mapstructure:"https_port"`
		// RequestTimeout is the default nginx request timeout in seconds,
		// overridable per project via .lerd.yaml request_timeout. Zero falls
		// back to nginx's own 60s default; read it via RequestTimeoutSeconds.
		RequestTimeout int `yaml:"request_timeout,omitempty" mapstructure:"request_timeout"`
	} `yaml:"nginx" mapstructure:"nginx"`
	DNS struct {
		// Enabled=false skips lerd-dns, mkcert CA, sudoers, and resolver
		// config; sites use *.localhost (RFC 6761). HTTPS is unavailable
		// in that mode. Default true preserves historical behaviour.
		Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
		TLD     string `yaml:"tld"     mapstructure:"tld"`
		// Upstream pins the upstream DNS servers dnsmasq forwards
		// non-.test queries to. When empty, lerd auto-detects them from
		// the system resolver. Set this when auto-detection picks the
		// wrong servers (e.g. systemd-resolved fallbacks instead of your
		// LAN resolver). Accepts plain IPs; #port is allowed.
		Upstream []string `yaml:"upstream,omitempty" mapstructure:"upstream"`
	} `yaml:"dns" mapstructure:"dns"`
	LAN struct {
		// Exposed controls whether lerd's services are reachable from
		// other devices on the local network. When false (the default,
		// safe-on-coffee-shop-wifi state) every container PublishPort is
		// rewritten to bind 127.0.0.1, lerd-ui binds 127.0.0.1:7073, and
		// the lerd-dns-forwarder is stopped. When true, container ports
		// bind 0.0.0.0, lerd-ui binds 0.0.0.0:7073, dnsmasq is rewritten
		// to answer .test queries with the host's LAN IP, and the
		// userspace lerd-dns-forwarder runs to bridge LAN-IP:5300 to the
		// loopback-only DNS container.
		//
		// Toggled via `lerd lan:expose on/off`. The previous standalone
		// `dns:expose` flag was folded in here because there is no
		// meaningful state where the DNS resolver answers the LAN but
		// the actual services don't.
		Exposed bool `yaml:"exposed,omitempty" mapstructure:"exposed"`
	} `yaml:"lan,omitempty" mapstructure:"lan"`
	Autostart struct {
		// Disabled controls whether lerd boots itself at login. The
		// zero value (false) means lerd autostarts as it always has:
		// every lerd-* container quadlet ships with its [Install]
		// section, the podman generator wires it into
		// default.target.wants on every daemon-reload, and the
		// lerd-ui / lerd-watcher / per-site worker units are enabled.
		// Setting this to true makes WriteQuadletDiff strip the
		// [Install] section before write (so the generator stops
		// emitting wants symlinks), disables ui/watcher and every
		// per-site worker, and stops them. Toggled via
		// `lerd autostart enable / disable` and the dashboard / tray
		// switches.
		//
		// Inverted form (Disabled rather than Enabled) so the YAML zero
		// value preserves the historical autostart-on behaviour for
		// every existing install — users who never touch the toggle
		// see no change.
		Disabled bool `yaml:"disabled,omitempty" mapstructure:"disabled"`
	} `yaml:"autostart,omitempty" mapstructure:"autostart"`
	UI struct {
		// RemoteControl gates non-loopback access to the lerd dashboard.
		// Empty PasswordHash = disabled = LAN clients get 403. With a hash
		// set, LAN clients must present matching HTTP Basic auth. Loopback
		// (127.0.0.1, ::1) always bypasses both checks.
		Username     string `yaml:"username,omitempty" mapstructure:"username"`
		PasswordHash string `yaml:"password_hash,omitempty" mapstructure:"password_hash"`
	} `yaml:"ui,omitempty" mapstructure:"ui"`
	Workers struct {
		// ExecMode controls how framework workers (queue, schedule, horizon,
		// reverb, custom) are launched on macOS. "exec" (default) wraps a
		// single `podman exec` per worker in a dedup guard and lets launchd
		// supervise that process, matching Linux's lower-memory behaviour.
		// "container" runs each worker as its own detached container, which
		// costs more memory per worker but makes the podman supervisor
		// boundary 1:1 and sidesteps the SSH-bridge hiccups that can
		// otherwise produce phantom or duplicate workers.
		//
		// The field is ignored on Linux, which always runs workers as
		// `podman exec` into the shared FPM container (systemd is a
		// dependable supervisor there). Use WorkerExecMode() to read the
		// effective value.
		ExecMode string `yaml:"exec_mode,omitempty" mapstructure:"exec_mode"`
	} `yaml:"workers,omitempty" mapstructure:"workers"`
	Dumps struct {
		// Enabled is the single switch for the whole debug window: the dump
		// bridge AND the lerd_devtools collector (queries, mail, views, events,
		// jobs, http). Both the bridge and the extension read one runtime
		// sentinel (`enabled.flag`); their PHP/ini assets are always mounted
		// regardless of this flag, so what Enabled controls is just that
		// sentinel — touch = capture, missing = fast no-op, no FPM restart.
		// Toggled via `lerd dump on/off` or the dashboard Debug view.
		Enabled bool `yaml:"enabled,omitempty" mapstructure:"enabled"`
		// Passthrough controls whether dump()/dd() ALSO emit to the response
		// while the bridge is enabled. False (default) means captured-only:
		// the dashboard is the single destination and the response stays
		// clean (matching Herd's behaviour). True forwards each call through
		// Symfony's stock VarDumper handler after capture, useful as a
		// safety net when lerd-ui isn't running. No effect when Enabled is
		// false — without the bridge, dump() behaves exactly as Symfony
		// ships it.
		Passthrough bool `yaml:"passthrough,omitempty" mapstructure:"passthrough"`
	} `yaml:"dumps,omitempty" mapstructure:"dumps"`
	Devtools struct {
		// Workers includes long-running queue/scheduler worker queries in
		// capture. Off by default because their constant polling floods the
		// buffer; toggled from the dashboard "Show worker queries" checkbox.
		// The collector's enable state is shared with the debug bridge: one
		// sentinel (enabled.flag) and one config flag (Dumps.Enabled) arm both,
		// so there is no separate devtools enable toggle.
		Workers bool `yaml:"workers,omitempty" mapstructure:"workers"`
	} `yaml:"devtools,omitempty" mapstructure:"devtools"`
	Profiler struct {
		// Enabled toggles the SPX profiler globally. When on, nginx injects
		// SPX_ENABLED into every PHP-FPM site's requests so each is profiled.
		// Toggled via `lerd profile on/off` and the dashboard Profiler view.
		Enabled bool `yaml:"enabled,omitempty" mapstructure:"enabled"`
	} `yaml:"profiler,omitempty" mapstructure:"profiler"`
	Notifications struct {
		// Disabled globally mutes the notifier (WebSocket banners + Web
		// Push fanout). Inverted form so the zero value keeps existing
		// installs on. Toggled via `lerd notify on/off` and the tray.
		Disabled bool `yaml:"disabled,omitempty" mapstructure:"disabled"`
		// Target selects the delivery sink: "browser" (WebSocket + Web
		// Push) or "native" (the daemon posts to org.freedesktop.Notifications
		// and Web Push is suppressed). Empty resolves to browser so upgrades
		// are unchanged; fresh Linux installs seed "native".
		Target string `yaml:"target,omitempty" mapstructure:"target"`
		// Kinds is the per-category on/off for the native sink. Browser prefs
		// live per-device in the page; native has no device, so the daemon
		// reads category filtering from here. Absent keys use the built-in
		// default (everything on except the noisy dump kind).
		Kinds map[string]bool `yaml:"kinds,omitempty" mapstructure:"kinds"`
	} `yaml:"notifications,omitempty" mapstructure:"notifications"`
	Tray struct {
		// HighContrastIcon swaps the running tray icon for a single green glyph
		// that reads on any panel, instead of the light/dark swap that guesses
		// wrong on mixed themes like KDE Breeze Twilight. Off by default so the
		// zero value keeps the theme-adaptive icon; toggled via `lerd tray icon`
		// and the tray menu.
		HighContrastIcon bool `yaml:"high_contrast_icon,omitempty" mapstructure:"high_contrast_icon"`
	} `yaml:"tray,omitempty" mapstructure:"tray"`
	HostProxy struct {
		// Disabled refuses to set up or start any host-proxy dev-server unit,
		// for users who never want lerd supervising a process on the host.
		// Inverted so the zero value keeps the feature available.
		Disabled bool `yaml:"disabled,omitempty" mapstructure:"disabled"`
		// SkipConfirmation runs a newly-linked host-proxy command without the
		// interactive "start this on your host?" prompt. Off by default so a
		// command from a cloned repo never runs unconfirmed.
		SkipConfirmation bool `yaml:"skip_confirmation,omitempty" mapstructure:"skip_confirmation"`
	} `yaml:"host_proxy,omitempty" mapstructure:"host_proxy"`
	HostCommands struct {
		// Disabled refuses to run any project-supplied host command or host
		// worker (custom_workers / commands from a project .lerd.yaml). Inverted
		// so the zero value keeps the feature available.
		Disabled bool `yaml:"disabled,omitempty" mapstructure:"disabled"`
		// SkipConfirmation runs project-supplied host commands and workers without
		// the interactive confirm. Off by default so a command from a cloned repo
		// never runs unconfirmed.
		SkipConfirmation bool `yaml:"skip_confirmation,omitempty" mapstructure:"skip_confirmation"`
	} `yaml:"host_commands,omitempty" mapstructure:"host_commands"`
	IdleSuspend struct {
		// Enabled turns on activity-driven worker suspension: when a site sees
		// no activity for Timeout, its suspendable workers (queue, horizon, ...)
		// are gracefully stopped and resumed on the next request. Off by default
		// so a quiet dev box reclaims worker memory only once the user opts in.
		// Idle-suspend is a single global policy, not configured per site.
		Enabled bool `yaml:"enabled,omitempty" mapstructure:"enabled"`
		// Timeout is how long a site must be idle before its workers suspend, as
		// a Go duration string ("30m"). Empty or unparseable falls back to
		// DefaultIdleSuspendTimeout; read it via IdleSuspendTimeout.
		Timeout string `yaml:"timeout,omitempty" mapstructure:"timeout"`
	} `yaml:"idle_suspend,omitempty" mapstructure:"idle_suspend"`
	// AutoCleanup lets the watcher periodically reclaim orphaned lerd images
	// (safe tier only, never service images). On by default; set false to turn
	// off the daily sweep. Read it via AutoCleanupEnabled for nil-safety.
	AutoCleanup       bool     `yaml:"auto_cleanup"       mapstructure:"auto_cleanup"`
	ParkedDirectories []string `yaml:"parked_directories" mapstructure:"parked_directories"`
	// Mounts are extra host paths the user opts in to bind-mounting into the
	// PHP-FPM and nginx containers at the same location, on top of $HOME and the
	// parked/linked-site paths. Unlike those, a mount may point at a normally
	// excluded system tree (e.g. a scratch root under /tmp for agent workflows):
	// listing it here is the explicit "yes, mount this" the ephemeral denylist
	// otherwise withholds. Same-location only, so an in-container path matches its
	// host path and `lerd php` can chdir into it.
	Mounts   []string                 `yaml:"mounts,omitempty"   mapstructure:"mounts"`
	Services map[string]ServiceConfig `yaml:"services"           mapstructure:"services"`
	// Workspaces group sites for display only, in the web UI and the TUI. See
	// workspaces.go; they never affect how a site is served.
	Workspaces []Workspace `yaml:"workspaces,omitempty" mapstructure:"workspaces"`
}

// AutoCleanupEnabled reports whether the watcher's periodic image cleanup is on.
// Nil-safe and defaults on, matching defaultConfig, so an unconfigured install
// reclaims orphaned images on its own.
func (c *GlobalConfig) AutoCleanupEnabled() bool {
	return c == nil || c.AutoCleanup
}

// DefaultIdleSuspendTimeout is how long a site stays idle before its
// suspendable workers are stopped, when no explicit timeout is configured.
const DefaultIdleSuspendTimeout = 30 * time.Minute

// IdleSuspendTimeout returns the effective global idle timeout, falling back to
// DefaultIdleSuspendTimeout when unset, unparseable, or non-positive.
func (c *GlobalConfig) IdleSuspendTimeout() time.Duration {
	if c.IdleSuspend.Timeout != "" {
		if d, err := time.ParseDuration(c.IdleSuspend.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return DefaultIdleSuspendTimeout
}

// DefaultRequestTimeout is nginx's built-in fastcgi/proxy read-timeout default
// (seconds), used when neither the project nor the global config sets one.
const DefaultRequestTimeout = 60

// RequestTimeoutSeconds returns the effective global nginx request timeout in
// seconds, falling back to nginx's 60s default when unset or non-positive.
func (c *GlobalConfig) RequestTimeoutSeconds() int {
	if c.Nginx.RequestTimeout > 0 {
		return c.Nginx.RequestTimeout
	}
	return DefaultRequestTimeout
}

// Worker exec-mode constants. `exec` is the default on every platform;
// `container` is available as an opt-in on macOS for users who prefer the
// reliability of per-worker containers over the memory savings of
// podman-exec into the shared FPM container.
const (
	WorkerExecModeExec      = "exec"
	WorkerExecModeContainer = "container"
)

// WorkerExecMode returns the effective worker exec mode for the current
// platform. Invalid or empty configured values normalise to "exec".
func (c *GlobalConfig) WorkerExecMode() string {
	switch c.Workers.ExecMode {
	case WorkerExecModeContainer:
		return WorkerExecModeContainer
	}
	return WorkerExecModeExec
}

// DNSManaged reports whether lerd is managing local DNS, which is the
// prerequisite for HTTPS (mkcert CA, *.test resolution). A nil receiver counts
// as managed, matching how the rest of the codebase treats an absent config
// (an unconfigured install behaves as if DNS is enabled). It is the single
// predicate the install wizard, `lerd secure`, and the cert layer all share.
func (c *GlobalConfig) DNSManaged() bool {
	return c == nil || c.DNS.Enabled
}

func defaultConfig() *GlobalConfig {
	cfg := &GlobalConfig{}
	cfg.PHP.DefaultVersion = "8.5"
	cfg.Node.DefaultVersion = "22"
	cfg.Nginx.HTTPPort = 80
	cfg.Nginx.HTTPSPort = 443
	cfg.DNS.Enabled = true
	cfg.DNS.TLD = "test"
	cfg.AutoCleanup = true

	home, _ := os.UserHomeDir()
	cfg.ParkedDirectories = []string{home + "/Lerd"}

	// Hydrate the per-service defaults from each default-preset YAML so the
	// preset is the single source of truth for image, host port and identity.
	// Image overrides users have written into ~/.config/lerd/config.yaml are
	// merged on top by viper after this point in LoadGlobal.
	cfg.Services = map[string]ServiceConfig{}
	for _, name := range DefaultPresetNames() {
		svc, err := DefaultPresetMeta(name)
		if err != nil {
			continue
		}
		entry := ServiceConfig{Enabled: true, Port: firstHostPort(svc.Ports)}
		// Skip the Image seed for track_latest presets so EnsureDefaultPresetQuadlet
		// can detect "fresh install, no user pin" and resolve the actual current
		// upstream tag at install time. Existing users' saved Image overrides
		// continue to win via viper merge.
		if p, _ := LoadPreset(name); p == nil || !p.TrackLatest {
			entry.Image = svc.Image
		}
		cfg.Services[name] = entry
	}
	return cfg
}

// HostPorts returns every host port this service entry actually binds: its
// effective primary (the PublishedPort override when set, else the preset-default
// Port) plus the host side of each ExtraPorts mapping. Single source for the
// serviceops port-ownership guard and the host-proxy dev server allocator, which
// previously parsed these out separately and drifted (the guard even mis-read the
// host side of an "ip:host:container" extra mapping). The default Port is NOT
// reported once an override moves the service off it: the quadlet publishes only
// the override, so the freed default must be reassignable to another service.
func (s ServiceConfig) HostPorts() []int {
	var ports []int
	primary := s.Port
	if s.PublishedPort > 0 {
		primary = s.PublishedPort
	}
	if primary > 0 {
		ports = append(ports, primary)
	}
	for _, h := range s.PublishedPorts {
		if h > 0 {
			ports = append(ports, h)
		}
	}
	for _, ep := range s.ExtraPorts {
		if n := MappingHostPort(ep); n > 0 {
			ports = append(ports, n)
		}
	}
	return ports
}

// HostPortFor returns the effective host port this service publishes for the
// mapping whose container-internal port is containerPort, given that mapping's
// preset default host side defaultHost and whether it is the primary (index-0)
// mapping. A per-port override in PublishedPorts wins; else the legacy primary
// PublishedPort applies to the primary mapping; else the preset default stands.
func (s ServiceConfig) HostPortFor(containerPort, defaultHost int, isPrimary bool) int {
	if v, ok := s.PublishedPorts[containerPort]; ok && v > 0 {
		return v
	}
	if isPrimary && s.PublishedPort > 0 {
		return s.PublishedPort
	}
	return defaultHost
}

// MappingHostPort extracts the host port from a podman port mapping: a bare host
// port "3411", "3411:3306", or "127.0.0.1:3411:3306", with an optional "/tcp"
// suffix. Returns 0 when no valid host port is present.
func MappingHostPort(mapping string) int {
	parts := strings.Split(mapping, ":")
	host := parts[0]
	if len(parts) > 1 {
		host = parts[len(parts)-2]
	}
	host = strings.SplitN(host, "/", 2)[0]
	n, _ := strconv.Atoi(strings.TrimSpace(host))
	return n
}

// mappingContainerPort extracts the container (internal) port from a podman port
// mapping — the last numeric segment after stripping an optional "/proto" suffix.
// A bare "3411" reports 3411, matching podman's host==container publish.
func mappingContainerPort(mapping string) int {
	body := strings.SplitN(mapping, "/", 2)[0]
	parts := strings.Split(body, ":")
	n, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	return n
}

// ReservedHostPorts returns every host port a lerd service may bind: each
// configured service entry's effective ports (HostPorts), every bundled preset's
// default ports (including optional presets not in the default set), and every
// installed custom service's ports. It is the single shared definition consumed
// by both the serviceops port-ownership guard and the host-proxy dev-server
// allocator so the two can never drift — the divergence that previously let the
// guard shift a built-in onto a stopped custom service's port and collide at boot,
// since the guard read only cfg.Services while the allocator also covered presets
// and customs.
func ReservedHostPorts() map[int]bool {
	reserved := map[int]bool{}
	cfg, _ := LoadGlobal()
	add := func(n int) {
		if n > 0 {
			reserved[n] = true
		}
	}
	// reserveDefaults reserves a service's default mappings, resolving each through
	// the service's recorded published override so a moved primary or secondary port
	// reserves its NEW port and frees the default, instead of pinning the vacated
	// default reserved forever (which would keep the guard and the host-proxy
	// allocator from ever reusing it). Matches HostPorts()'s freed-default contract.
	reserveDefaults := func(name string, mappings []string) {
		var svc ServiceConfig
		configured := false
		if cfg != nil {
			svc, configured = cfg.Services[name]
		}
		for i, m := range mappings {
			host := MappingHostPort(m)
			if host <= 0 {
				continue
			}
			if configured {
				host = svc.HostPortFor(mappingContainerPort(m), host, i == 0)
			}
			add(host)
		}
	}
	if cfg != nil {
		for _, svc := range cfg.Services {
			for _, p := range svc.HostPorts() {
				add(p)
			}
		}
		for _, specs := range cfg.PHP.FPMPorts {
			for _, m := range specs {
				add(MappingHostPort(m))
			}
		}
	}
	if presets, err := ListPresets(); err == nil {
		for _, meta := range presets {
			if p, err := LoadPreset(meta.Name); err == nil {
				reserveDefaults(meta.Name, p.Ports)
			}
		}
	}
	if customs, err := ListCustomServices(); err == nil {
		for _, svc := range customs {
			reserveDefaults(svc.Name, svc.Ports)
		}
	}
	return reserved
}

// FPMPortsFor returns the extra published port mappings recorded for a PHP
// version's shared FPM container, or nil when none are configured. Read by the
// FPM quadlet renderer so a `lerd shell` process on one of these ports is
// reachable from the host, and by the port-shift guard to skip a version's own
// ports when relocating a colliding one.
func FPMPortsFor(version string) []string {
	cfg, err := LoadGlobal()
	if err != nil || cfg == nil || cfg.PHP.FPMPorts == nil {
		return nil
	}
	return cfg.PHP.FPMPorts[version]
}

// firstHostPort returns the host-side port number from the first ports entry,
// e.g. "3306:3306" → 3306. Used by defaultConfig to populate ServiceConfig.Port
// without mirroring the YAML port literals in code.
func firstHostPort(ports []string) int {
	if len(ports) == 0 {
		return 0
	}
	first := ports[0]
	if i := strings.Index(first, ":"); i >= 0 {
		first = first[:i]
	}
	n, _ := strconv.Atoi(first)
	return n
}

// globalCache memoises the last LoadGlobal result keyed on config.yaml's
// mtime+size. The daemon's snapshot path used to call LoadGlobal hundreds of
// times per rebuild (one per site, transitively), and each call re-parsed
// every preset YAML via defaultConfig — pprof showed yaml.Unmarshal as the
// dominant CPU cost. The cache returns a deep copy so callers can mutate the
// returned struct without poisoning the cache.
var (
	globalCacheMu sync.Mutex
	globalCache   *GlobalConfig
	globalCacheAt time.Time
	globalCacheSz int64
)

// invalidateGlobalCache drops the cached config so the next LoadGlobal re-reads
// from disk. Called from SaveGlobal so writes are visible immediately.
func invalidateGlobalCache() {
	globalCacheMu.Lock()
	globalCache = nil
	globalCacheAt = time.Time{}
	globalCacheSz = 0
	globalCacheMu.Unlock()
}

// EffectiveTLD returns the configured DNS TLD, falling back to "test" when the
// global config can't be loaded or leaves it empty. Single source of truth for
// the many callers that need the active TLD with that default.
func EffectiveTLD() string {
	if cfg, err := LoadGlobal(); err == nil && cfg.DNS.TLD != "" {
		return cfg.DNS.TLD
	}
	return "test"
}

// LoadGlobal reads config.yaml via viper, returning defaults if the file is absent.
func LoadGlobal() (*GlobalConfig, error) {
	cfgFile := GlobalConfigFile()

	var (
		statMtime time.Time
		statSize  int64
		statErr   error
	)
	if info, err := os.Stat(cfgFile); err == nil {
		statMtime = info.ModTime()
		statSize = info.Size()
	} else {
		statErr = err
	}

	globalCacheMu.Lock()
	if globalCache != nil && statErr == nil &&
		globalCacheAt.Equal(statMtime) && globalCacheSz == statSize {
		out := cloneGlobalConfig(globalCache)
		globalCacheMu.Unlock()
		return out, nil
	}
	globalCacheMu.Unlock()

	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigFile(cfgFile)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}

	// Runs before Unmarshal: the legacy per-version shape is a map where the
	// current one is a list, so decoding an old config would fail outright.
	migrateUnifiedPHPSets(v)

	cfg := defaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	migrateStaleServiceImages(cfg)

	if statErr == nil {
		globalCacheMu.Lock()
		globalCache = cloneGlobalConfig(cfg)
		globalCacheAt = statMtime
		globalCacheSz = statSize
		globalCacheMu.Unlock()
	}
	return cfg, nil
}

// cloneGlobalConfig returns a deep copy. Maps and slices are duplicated so
// callers cannot mutate the cached value.
func cloneGlobalConfig(in *GlobalConfig) *GlobalConfig {
	out := *in
	if in.PHP.XdebugEnabled != nil {
		out.PHP.XdebugEnabled = make(map[string]bool, len(in.PHP.XdebugEnabled))
		for k, v := range in.PHP.XdebugEnabled {
			out.PHP.XdebugEnabled[k] = v
		}
	}
	if in.PHP.XdebugMode != nil {
		out.PHP.XdebugMode = make(map[string]string, len(in.PHP.XdebugMode))
		for k, v := range in.PHP.XdebugMode {
			out.PHP.XdebugMode[k] = v
		}
	}
	if in.PHP.XdebugStart != nil {
		out.PHP.XdebugStart = make(map[string]string, len(in.PHP.XdebugStart))
		for k, v := range in.PHP.XdebugStart {
			out.PHP.XdebugStart[k] = v
		}
	}
	out.PHP.Extensions = slices.Clone(in.PHP.Extensions)
	out.PHP.Packages = slices.Clone(in.PHP.Packages)
	if in.PHP.Realised != nil {
		out.PHP.Realised = make(map[string]RealisedPHPSet, len(in.PHP.Realised))
		for k, v := range in.PHP.Realised {
			out.PHP.Realised[k] = RealisedPHPSet{
				Hash:       v.Hash,
				Extensions: slices.Clone(v.Extensions),
				Packages:   slices.Clone(v.Packages),
			}
		}
	}
	if in.PHP.ExtApkDeps != nil {
		out.PHP.ExtApkDeps = make(map[string][]string, len(in.PHP.ExtApkDeps))
		for k, v := range in.PHP.ExtApkDeps {
			cp := make([]string, len(v))
			copy(cp, v)
			out.PHP.ExtApkDeps[k] = cp
		}
	}
	if in.PHP.FPMPorts != nil {
		out.PHP.FPMPorts = make(map[string][]string, len(in.PHP.FPMPorts))
		for k, v := range in.PHP.FPMPorts {
			cp := make([]string, len(v))
			copy(cp, v)
			out.PHP.FPMPorts[k] = cp
		}
	}
	if in.ParkedDirectories != nil {
		out.ParkedDirectories = append([]string(nil), in.ParkedDirectories...)
	}
	if in.Mounts != nil {
		out.Mounts = append([]string(nil), in.Mounts...)
	}
	if in.DNS.Upstream != nil {
		out.DNS.Upstream = append([]string(nil), in.DNS.Upstream...)
	}
	if in.Services != nil {
		out.Services = make(map[string]ServiceConfig, len(in.Services))
		for k, v := range in.Services {
			cp := v
			if v.ExtraPorts != nil {
				cp.ExtraPorts = append([]string(nil), v.ExtraPorts...)
			}
			if v.PublishedPorts != nil {
				cp.PublishedPorts = make(map[int]int, len(v.PublishedPorts))
				for pk, pv := range v.PublishedPorts {
					cp.PublishedPorts[pk] = pv
				}
			}
			out.Services[k] = cp
		}
	}
	if in.Workspaces != nil {
		out.Workspaces = make([]Workspace, len(in.Workspaces))
		for i, w := range in.Workspaces {
			cp := w
			cp.Sites = append([]string(nil), w.Sites...)
			out.Workspaces[i] = cp
		}
	}
	return &out
}

// staleServiceImages maps service name → list of historical default images
// that earlier lerd releases persisted into user configs. When LoadGlobal
// finds one of these on disk it transparently replaces it with the current
// default from defaultConfig() so users picking up the upgrade automatically
// move onto the new image (e.g. postgres → postgis/postgis for PostGIS
// support) without having to hand-edit ~/.config/lerd/config.yaml.
var staleServiceImages = map[string][]string{
	"mysql": {
		"mysql:8.0",
	},
	"redis": {
		"redis:7-alpine",
	},
	"postgres": {
		"postgres:16-alpine",
		"docker.io/library/postgres:16-alpine",
		"docker.io/postgres:16-alpine",
		"postgis/postgis:16-3.5-alpine",
	},
	// meilisearch deliberately omitted: every minor bump breaks data-dir
	// compatibility, so silently upgrading existing v1.7 users to v1.42
	// would crash their running container. New installs pick up the latest
	// minor through defaultConfig; existing users keep their pinned image.
	"rustfs": {
		"rustfs/rustfs:latest",
	},
	"mailpit": {
		"axllent/mailpit:latest",
	},
}

func migrateStaleServiceImages(cfg *GlobalConfig) {
	if cfg == nil || cfg.Services == nil {
		return
	}
	defaults := defaultConfig().Services
	changed := false
	for name, stale := range staleServiceImages {
		svc, ok := cfg.Services[name]
		if !ok {
			continue
		}
		def, hasDefault := defaults[name]
		if !hasDefault {
			continue
		}
		// Skip migration for track_latest presets where defaultConfig has no
		// concrete image: rewriting to "" would land the user in the
		// fresh-install path on next start, silently bumping their data dir
		// across major-line boundaries (e.g. mysql:8.0 → 8.4 forward upgrade).
		if def.Image == "" {
			continue
		}
		for _, s := range stale {
			if svc.Image == s {
				svc.Image = def.Image
				cfg.Services[name] = svc
				changed = true
				break
			}
		}
	}
	if changed {
		_ = SaveGlobal(cfg)
	}
}

// IsXdebugEnabled returns true if Xdebug is enabled for the given PHP version.
func (c *GlobalConfig) IsXdebugEnabled(version string) bool {
	return c.GetXdebugMode(version) != ""
}

// GetXdebugMode returns the configured Xdebug mode for version, or "" when
// disabled. Entries in the legacy xdebug_enabled map (no explicit mode) are
// treated as mode "debug" so configs written by older lerd builds keep the
// same behaviour they had before per-mode support existed.
func (c *GlobalConfig) GetXdebugMode(version string) string {
	if m, ok := c.PHP.XdebugMode[version]; ok && m != "" {
		return m
	}
	if c.PHP.XdebugEnabled[version] {
		return "debug"
	}
	return ""
}

// SetXdebug enables (mode "debug") or disables Xdebug for version. Use
// SetXdebugMode directly when a non-default mode is wanted.
func (c *GlobalConfig) SetXdebug(version string, enabled bool) {
	if !enabled {
		c.SetXdebugMode(version, "")
		return
	}
	c.SetXdebugMode(version, "debug")
}

// SetXdebugMode sets the Xdebug mode for version. Empty mode disables Xdebug.
// Both the modern xdebug_mode map and the legacy xdebug_enabled map are kept
// in sync so downgrades don't silently flip state.
func (c *GlobalConfig) SetXdebugMode(version, mode string) {
	if c.PHP.XdebugEnabled == nil {
		c.PHP.XdebugEnabled = map[string]bool{}
	}
	if c.PHP.XdebugMode == nil {
		c.PHP.XdebugMode = map[string]string{}
	}
	if mode == "" {
		delete(c.PHP.XdebugEnabled, version)
		delete(c.PHP.XdebugMode, version)
		return
	}
	c.PHP.XdebugEnabled[version] = true
	c.PHP.XdebugMode[version] = mode
}

// GetXdebugStart returns the xdebug.start_with_request value for version,
// defaulting to "yes" (connect on every request) when unset.
func (c *GlobalConfig) GetXdebugStart(version string) string {
	if v, ok := c.PHP.XdebugStart[version]; ok && v != "" {
		return v
	}
	return "yes"
}

// SetXdebugStart records the xdebug.start_with_request value for version. An
// empty value (or "yes", the default) clears the entry so the config stays lean.
func (c *GlobalConfig) SetXdebugStart(version, value string) {
	if value == "" || value == "yes" {
		delete(c.PHP.XdebugStart, version)
		return
	}
	if c.PHP.XdebugStart == nil {
		c.PHP.XdebugStart = map[string]string{}
	}
	c.PHP.XdebugStart[version] = value
}

// GetExtensions returns the declared custom extension set, applied to every PHP image.
func (c *GlobalConfig) GetExtensions() []string {
	return c.PHP.Extensions
}

// AddExtension adds ext to the declared set (no-op if already present).
func (c *GlobalConfig) AddExtension(ext string) {
	if !slices.Contains(c.PHP.Extensions, ext) {
		c.PHP.Extensions = append(c.PHP.Extensions, ext)
	}
}

// RemoveExtension drops ext from the declared set, along with any extra apk
// deps recorded for it, which are dead weight once nothing declares it.
func (c *GlobalConfig) RemoveExtension(ext string) {
	c.PHP.Extensions = slices.DeleteFunc(c.PHP.Extensions, func(e string) bool { return e == ext })
	delete(c.PHP.ExtApkDeps, ext)
	if len(c.PHP.ExtApkDeps) == 0 {
		c.PHP.ExtApkDeps = nil
	}
}

// GetPackages returns the declared extra Alpine package set.
func (c *GlobalConfig) GetPackages() []string {
	return c.PHP.Packages
}

// AddPackage adds pkg to the declared set (no-op if already present).
func (c *GlobalConfig) AddPackage(pkg string) {
	if !slices.Contains(c.PHP.Packages, pkg) {
		c.PHP.Packages = append(c.PHP.Packages, pkg)
	}
}

// RemovePackage drops pkg from the declared set.
func (c *GlobalConfig) RemovePackage(pkg string) {
	c.PHP.Packages = slices.DeleteFunc(c.PHP.Packages, func(p string) bool { return p == pkg })
}

// GetRealised returns what the given version's image actually loaded at its
// last build. A zero value means lerd has not built that version yet.
func (c *GlobalConfig) GetRealised(version string) RealisedPHPSet {
	return c.PHP.Realised[version]
}

// SetRealised records what a version's image loaded, verified after its build.
func (c *GlobalConfig) SetRealised(version string, set RealisedPHPSet) {
	if c.PHP.Realised == nil {
		c.PHP.Realised = map[string]RealisedPHPSet{}
	}
	c.PHP.Realised[version] = set
}

// MissingFromImage returns the declared entries that the given version's image
// did not load. A version with no recorded build returns nothing: lerd knows
// nothing about it yet, and reporting everything as missing would be a lie in
// the opposite direction.
func (c *GlobalConfig) MissingFromImage(version string, declared []string) []string {
	realised, ok := c.PHP.Realised[version]
	if !ok {
		return nil
	}
	var missing []string
	for _, d := range declared {
		if !slices.Contains(realised.Extensions, d) && !slices.Contains(realised.Packages, d) {
			missing = append(missing, d)
		}
	}
	return missing
}

// GetExtApkDeps returns the user-configured extra Alpine packages for ext.
func (c *GlobalConfig) GetExtApkDeps(ext string) []string {
	if c.PHP.ExtApkDeps == nil {
		return nil
	}
	return c.PHP.ExtApkDeps[ext]
}

// AllExtApkDeps returns the full user-configured extension → apk deps map.
func (c *GlobalConfig) AllExtApkDeps() map[string][]string {
	return c.PHP.ExtApkDeps
}

// SetExtApkDeps records (or clears, when deps is empty) the extra Alpine
// packages needed to build ext.
func (c *GlobalConfig) SetExtApkDeps(ext string, deps []string) {
	if len(deps) == 0 {
		delete(c.PHP.ExtApkDeps, ext)
		if len(c.PHP.ExtApkDeps) == 0 {
			c.PHP.ExtApkDeps = nil
		}
		return
	}
	if c.PHP.ExtApkDeps == nil {
		c.PHP.ExtApkDeps = map[string][]string{}
	}
	cp := make([]string, len(deps))
	copy(cp, deps)
	c.PHP.ExtApkDeps[ext] = cp
}

// IsDumpsEnabled reports whether the lerd debug bridge is on for all PHP
// versions. The toggle is global because the bridge file is a single,
// version-agnostic asset bind-mounted into every FPM container.
func (c *GlobalConfig) IsDumpsEnabled() bool {
	return c.Dumps.Enabled
}

// SetDumpsEnabled flips the debug bridge toggle. Persist via SaveGlobal and
// run dumpsops.Apply to actually rewrite the FPM quadlets.
func (c *GlobalConfig) SetDumpsEnabled(enabled bool) {
	c.Dumps.Enabled = enabled
}

// IsDevtoolsWorkers reports whether queue/scheduler worker queries are captured.
func (c *GlobalConfig) IsDevtoolsWorkers() bool {
	return c.Devtools.Workers
}

// SetDevtoolsWorkers flips worker-query capture. Persist via SaveGlobal and run
// devtoolsops.SetWorkers to touch the runtime sentinel.
func (c *GlobalConfig) SetDevtoolsWorkers(enabled bool) {
	c.Devtools.Workers = enabled
}

// IsProfilerEnabled reports whether the SPX profiler is globally armed.
func (c *GlobalConfig) IsProfilerEnabled() bool {
	return c.Profiler.Enabled
}

// IsDumpsPassthrough reports whether the bridge should also forward each
// captured dump to Symfony's stock VarDumper handler (response output).
// Always false in effect when the bridge itself is disabled.
func (c *GlobalConfig) IsDumpsPassthrough() bool {
	return c.Dumps.Passthrough
}

// SetDumpsPassthrough flips the passthrough flag. Persist via SaveGlobal
// and follow up with a `lerd-php*-fpm` restart so the new ini value takes
// effect (PHP reads ini directives at FPM startup, not per request).
func (c *GlobalConfig) SetDumpsPassthrough(enabled bool) {
	c.Dumps.Passthrough = enabled
}

// IsNotificationsEnabled reports whether the global notifier is allowed
// to fan out (WebSocket banners + Web Push). Inverted storage so existing
// installs default to enabled.
func (c *GlobalConfig) IsNotificationsEnabled() bool {
	return !c.Notifications.Disabled
}

// SetNotificationsEnabled flips the global notifier toggle. Persist via
// SaveGlobal; dispatchNotification re-reads the flag on every event.
func (c *GlobalConfig) SetNotificationsEnabled(enabled bool) {
	c.Notifications.Disabled = !enabled
}

// Notification delivery sinks. Browser is the WebSocket + Web Push path;
// Native posts straight to the desktop notification daemon.
const (
	NotifyTargetBrowser = "browser"
	NotifyTargetNative  = "native"
)

// NotificationTarget resolves the delivery sink. Any unrecognised or empty
// value falls back to browser so existing installs and future values stay safe.
func (c *GlobalConfig) NotificationTarget() string {
	if c.Notifications.Target == NotifyTargetNative {
		return NotifyTargetNative
	}
	return NotifyTargetBrowser
}

// SetNotificationTarget stores the delivery sink. Persist via SaveGlobal.
func (c *GlobalConfig) SetNotificationTarget(target string) {
	c.Notifications.Target = target
}

// NotifyKinds is the canonical set of notification categories, mirroring the
// web UI's ALL_KINDS, in display order.
var NotifyKinds = []string{
	"mail", "worker_failed", "op_done", "update_available", "nplusone", "slow_route", "dump",
}

// nativeKindDefault is the built-in on/off for a category the user has not set.
// Mirrors the web defaults: everything on except the noisy dump kind.
func nativeKindDefault(kind string) bool {
	return kind != "dump"
}

// NativeKindEnabled reports whether a category fires on the native sink.
func (c *GlobalConfig) NativeKindEnabled(kind string) bool {
	if c.Notifications.Kinds != nil {
		if v, ok := c.Notifications.Kinds[kind]; ok {
			return v
		}
	}
	return nativeKindDefault(kind)
}

// SetNativeKind sets a category's native on/off. Persist via SaveGlobal.
func (c *GlobalConfig) SetNativeKind(kind string, on bool) {
	if c.Notifications.Kinds == nil {
		c.Notifications.Kinds = map[string]bool{}
	}
	c.Notifications.Kinds[kind] = on
}

// EffectiveNativeKinds resolves the on/off for every known category.
func (c *GlobalConfig) EffectiveNativeKinds() map[string]bool {
	out := make(map[string]bool, len(NotifyKinds))
	for _, k := range NotifyKinds {
		out[k] = c.NativeKindEnabled(k)
	}
	return out
}

// IsHighContrastTrayIcon reports whether the tray should show the always-visible
// green running icon instead of the theme-adaptive light/dark one.
func (c *GlobalConfig) IsHighContrastTrayIcon() bool {
	return c.Tray.HighContrastIcon
}

// SetHighContrastTrayIcon flips the tray running-icon style. Persist via
// SaveGlobal; the tray re-reads it on every poll.
func (c *GlobalConfig) SetHighContrastTrayIcon(enabled bool) {
	c.Tray.HighContrastIcon = enabled
}

// NodeManagedPref returns the persisted Node-management choice. set is false
// when the config predates the field, so callers fall back to the on-disk shim
// state instead of assuming a default.
func (c *GlobalConfig) NodeManagedPref() (val bool, set bool) {
	if c.Node.Managed == nil {
		return false, false
	}
	return *c.Node.Managed, true
}

// SetNodeManaged records the Node-management choice. Persist via SaveGlobal;
// the install/update flow reads it to keep the choice across updates.
func (c *GlobalConfig) SetNodeManaged(managed bool) {
	c.Node.Managed = &managed
}

// NodeManager returns the configured Node version manager, defaulting to "fnm"
// when unset so configs predating the field keep the bundled behaviour.
func (c *GlobalConfig) NodeManager() string {
	if c.Node.Manager == "" {
		return "fnm"
	}
	return c.Node.Manager
}

// SetNodeManager records which Node version manager lerd drives ("fnm" or
// "nvm"). Persist via SaveGlobal.
func (c *GlobalConfig) SetNodeManager(manager string) {
	c.Node.Manager = manager
}

// NodeNvmDir returns the persisted nvm install directory, or empty when unset.
func (c *GlobalConfig) NodeNvmDir() string {
	return c.Node.NvmDir
}

// SetNodeNvmDir records where nvm lives so daemons agree with the CLI. Pass
// empty to clear (fnm switch, or fall back to $NVM_DIR / ~/.nvm).
func (c *GlobalConfig) SetNodeNvmDir(dir string) {
	c.Node.NvmDir = dir
}

// SaveGlobal writes the configuration to config.yaml.
func SaveGlobal(cfg *GlobalConfig) error {
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	guardRealWrite(GlobalConfigFile())
	if err := os.WriteFile(GlobalConfigFile(), data, 0644); err != nil {
		return err
	}
	invalidateGlobalCache()
	return nil
}
