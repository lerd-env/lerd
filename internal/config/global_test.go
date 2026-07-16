package config

import (
	"os"
	"testing"
)

// setConfigDir points ConfigDir() and DataDir() at a temp directory.
func setConfigDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
}

// ── LoadGlobal ────────────────────────────────────────────────────────────────

func TestLoadGlobal_Defaults(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.PHP.DefaultVersion == "" {
		t.Error("expected a default PHP version")
	}
	if cfg.DNS.TLD == "" {
		t.Error("expected a default DNS TLD")
	}
	if !cfg.DNS.Enabled {
		t.Error("expected DNS.Enabled to default true")
	}
	if cfg.Nginx.HTTPPort == 0 {
		t.Error("expected a non-zero HTTP port")
	}
	if cfg.Nginx.HTTPSPort == 0 {
		t.Error("expected a non-zero HTTPS port")
	}
}

// A config returned by LoadGlobal must not share its PHP.Packages map with the
// cache: php:pkg's AddPackage/RemovePackage mutate the loaded copy in place
// before SaveGlobal, and that must not corrupt what later LoadGlobal calls see.
func TestLoadGlobal_PackagesNotAliasedWithCache(t *testing.T) {
	setConfigDir(t)

	seed, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	seed.AddPackage("vim")
	if err := SaveGlobal(seed); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	loaded, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	// Mutate the loaded copy without saving — must not reach the cache.
	loaded.AddPackage("htop")

	again, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	got := again.GetPackages()
	if len(got) != 1 || got[0] != "vim" {
		t.Errorf("cache corrupted by an unsaved mutation: GetPackages = %v, want [vim]", got)
	}
}

func TestSaveLoadGlobal_RoundTrip(t *testing.T) {
	setConfigDir(t)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}

	cfg.PHP.DefaultVersion = "8.2"
	cfg.Node.DefaultVersion = "20"
	cfg.DNS.TLD = "local"
	cfg.Nginx.HTTPPort = 8080

	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after save: %v", err)
	}
	if got.PHP.DefaultVersion != "8.2" {
		t.Errorf("PHP.DefaultVersion = %q, want %q", got.PHP.DefaultVersion, "8.2")
	}
	if got.Node.DefaultVersion != "20" {
		t.Errorf("Node.DefaultVersion = %q, want %q", got.Node.DefaultVersion, "20")
	}
	if got.DNS.TLD != "local" {
		t.Errorf("DNS.TLD = %q, want %q", got.DNS.TLD, "local")
	}
	if got.Nginx.HTTPPort != 8080 {
		t.Errorf("Nginx.HTTPPort = %d, want 8080", got.Nginx.HTTPPort)
	}
}

func TestNodeManagedPref(t *testing.T) {
	setConfigDir(t)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	// A fresh config predates the field, so the preference is unset and callers
	// fall back to the on-disk shim state.
	if _, set := cfg.NodeManagedPref(); set {
		t.Fatal("expected NodeManagedPref unset on a fresh config")
	}

	cfg.SetNodeManaged(false)
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after save: %v", err)
	}
	val, set := got.NodeManagedPref()
	if !set {
		t.Fatal("expected NodeManagedPref set after SetNodeManaged")
	}
	if val {
		t.Errorf("NodeManagedPref = true, want false (the opt-out must survive a round-trip)")
	}
}

// ── RequestTimeoutSeconds ─────────────────────────────────────────────────────

func TestRequestTimeoutSeconds_DefaultsTo60WhenUnset(t *testing.T) {
	cfg := &GlobalConfig{}
	if got := cfg.RequestTimeoutSeconds(); got != DefaultRequestTimeout {
		t.Errorf("RequestTimeoutSeconds = %d, want %d", got, DefaultRequestTimeout)
	}
}

func TestRequestTimeoutSeconds_HonoursConfiguredValue(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.Nginx.RequestTimeout = 300
	if got := cfg.RequestTimeoutSeconds(); got != 300 {
		t.Errorf("RequestTimeoutSeconds = %d, want 300", got)
	}
}

func TestRequestTimeoutSeconds_NonPositiveFallsBack(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.Nginx.RequestTimeout = -5
	if got := cfg.RequestTimeoutSeconds(); got != DefaultRequestTimeout {
		t.Errorf("RequestTimeoutSeconds = %d, want %d for non-positive", got, DefaultRequestTimeout)
	}
}

func TestSaveLoadGlobal_RequestTimeoutRoundTrip(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.Nginx.RequestTimeout = 240
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after save: %v", err)
	}
	if got.Nginx.RequestTimeout != 240 {
		t.Errorf("Nginx.RequestTimeout = %d, want 240", got.Nginx.RequestTimeout)
	}
}

// ── Cache ─────────────────────────────────────────────────────────────────────

func TestLoadGlobal_CacheReturnsIndependentCopy(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.DNS.TLD = "local"
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceConfig{}
	}
	cfg.Services["mutated"] = ServiceConfig{Enabled: true}

	again, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal #2: %v", err)
	}
	if again.DNS.TLD == "local" {
		t.Error("cached value should not reflect caller mutation of DNS.TLD")
	}
	if _, ok := again.Services["mutated"]; ok {
		t.Error("cached value should not reflect caller mutation of Services map")
	}
}

func TestLoadGlobal_CacheInvalidatedBySaveGlobal(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.DNS.TLD = "local"
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after save: %v", err)
	}
	if got.DNS.TLD != "local" {
		t.Errorf("after SaveGlobal, DNS.TLD = %q, want %q", got.DNS.TLD, "local")
	}
}

// ── Xdebug ────────────────────────────────────────────────────────────────────

func TestXdebug_Toggle(t *testing.T) {
	cfg := &GlobalConfig{}

	if cfg.IsXdebugEnabled("8.3") {
		t.Error("expected xdebug disabled by default")
	}

	cfg.SetXdebug("8.3", true)
	if !cfg.IsXdebugEnabled("8.3") {
		t.Error("expected xdebug enabled after SetXdebug(true)")
	}

	cfg.SetXdebug("8.3", false)
	if cfg.IsXdebugEnabled("8.3") {
		t.Error("expected xdebug disabled after SetXdebug(false)")
	}
}

func TestXdebug_IndependentVersions(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.SetXdebug("8.3", true)
	cfg.SetXdebug("8.4", false)

	if !cfg.IsXdebugEnabled("8.3") {
		t.Error("8.3 should still be enabled")
	}
	if cfg.IsXdebugEnabled("8.4") {
		t.Error("8.4 should remain disabled")
	}
}

func TestXdebug_ModeRoundtrip(t *testing.T) {
	cfg := &GlobalConfig{}

	cfg.SetXdebugMode("8.3", "coverage")
	if cfg.GetXdebugMode("8.3") != "coverage" {
		t.Errorf("GetXdebugMode = %q, want %q", cfg.GetXdebugMode("8.3"), "coverage")
	}
	if !cfg.IsXdebugEnabled("8.3") {
		t.Error("IsXdebugEnabled should be true when a mode is set")
	}

	cfg.SetXdebugMode("8.3", "debug,coverage")
	if cfg.GetXdebugMode("8.3") != "debug,coverage" {
		t.Errorf("GetXdebugMode = %q, want combo", cfg.GetXdebugMode("8.3"))
	}

	cfg.SetXdebugMode("8.3", "")
	if cfg.IsXdebugEnabled("8.3") {
		t.Error("empty mode should disable xdebug")
	}
}

func TestXdebug_StartRoundtrip(t *testing.T) {
	cfg := &GlobalConfig{}

	// Default is "yes" when unset.
	if got := cfg.GetXdebugStart("8.4"); got != "yes" {
		t.Errorf("default GetXdebugStart = %q, want yes", got)
	}

	cfg.SetXdebugStart("8.4", "trigger")
	if got := cfg.GetXdebugStart("8.4"); got != "trigger" {
		t.Errorf("GetXdebugStart = %q, want trigger", got)
	}

	// Setting back to the default clears the entry so the config stays lean.
	cfg.SetXdebugStart("8.4", "yes")
	if _, ok := cfg.PHP.XdebugStart["8.4"]; ok {
		t.Error(`setting "yes" should clear the stored entry`)
	}
	if got := cfg.GetXdebugStart("8.4"); got != "yes" {
		t.Errorf("after clear GetXdebugStart = %q, want yes", got)
	}
}

// Legacy configs (lerd <= 1.15.1) only wrote xdebug_enabled. GetXdebugMode
// must fall back to "debug" for those entries so upgrade keeps working.
func TestXdebug_LegacyEnabledFallsBackToDebug(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.PHP.XdebugEnabled = map[string]bool{"8.3": true}

	if got := cfg.GetXdebugMode("8.3"); got != "debug" {
		t.Errorf("legacy xdebug_enabled → GetXdebugMode = %q, want %q", got, "debug")
	}
	if !cfg.IsXdebugEnabled("8.3") {
		t.Error("IsXdebugEnabled should honour legacy flag")
	}
}

func TestXdebug_SetXdebugDefaultsToDebugMode(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.SetXdebug("8.3", true)
	if got := cfg.GetXdebugMode("8.3"); got != "debug" {
		t.Errorf("SetXdebug(true) → mode = %q, want %q", got, "debug")
	}
}

// ── Extensions ────────────────────────────────────────────────────────────────
// Add/remove/get of the declared sets is covered by TestPHPSetAccessors in
// php_sets_test.go, alongside the migration that produces them.

func TestExtApkDeps_SetGetClear(t *testing.T) {
	cfg := &GlobalConfig{}

	if deps := cfg.GetExtApkDeps("imap"); deps != nil {
		t.Errorf("expected nil deps, got %v", deps)
	}

	cfg.SetExtApkDeps("imap", []string{"imap-dev", "krb5-dev"})
	if got := cfg.GetExtApkDeps("imap"); len(got) != 2 || got[0] != "imap-dev" || got[1] != "krb5-dev" {
		t.Fatalf("expected [imap-dev krb5-dev], got %v", got)
	}
	if all := cfg.AllExtApkDeps(); len(all) != 1 {
		t.Errorf("expected 1 entry in AllExtApkDeps, got %v", all)
	}

	// SetExtApkDeps with empty deps clears the entry and nils the map.
	cfg.SetExtApkDeps("imap", nil)
	if cfg.GetExtApkDeps("imap") != nil || cfg.AllExtApkDeps() != nil {
		t.Errorf("empty SetExtApkDeps should clear the entry and the map")
	}
}

func TestExtApkDeps_DroppedWhenExtensionRemoved(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.AddExtension("ssh2")
	cfg.SetExtApkDeps("ssh2", []string{"libssh2-dev"})

	cfg.RemoveExtension("ssh2")
	if cfg.GetExtApkDeps("ssh2") != nil {
		t.Error("deps should be dropped once the extension is no longer declared")
	}
}

func TestExtApkDeps_DeepCopied(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.SetExtApkDeps("imap", []string{"imap-dev"})
	clone := cloneGlobalConfig(cfg)
	clone.SetExtApkDeps("imap", []string{"imap-dev", "krb5-dev"})
	if len(cfg.GetExtApkDeps("imap")) != 1 {
		t.Errorf("mutating the clone must not affect the original: %v", cfg.GetExtApkDeps("imap"))
	}
}

// A clone's per-service PublishedPorts map must not alias the original's, or a
// secondary-port override written into a loaded config would mutate the shared
// cache (risking a concurrent map read/write in lerd-ui).
func TestCloneGlobalConfig_PublishedPortsDeepCopied(t *testing.T) {
	cfg := &GlobalConfig{
		Services: map[string]ServiceConfig{
			"mailpit": {PublishedPorts: map[int]int{8025: 8025}},
		},
	}
	clone := cloneGlobalConfig(cfg)
	clone.Services["mailpit"].PublishedPorts[8025] = 9025
	if got := cfg.Services["mailpit"].PublishedPorts[8025]; got != 8025 {
		t.Errorf("mutating the clone must not affect the original: got %d, want 8025", got)
	}
}

func TestExtensions_AddIdempotent(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.AddExtension("redis")
	cfg.AddExtension("redis")

	if len(cfg.GetExtensions()) != 1 {
		t.Error("duplicate add should be a no-op")
	}
}

func TestExtensions_RemoveLastCleansSet(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.AddExtension("redis")
	cfg.RemoveExtension("redis")

	if exts := cfg.GetExtensions(); len(exts) != 0 {
		t.Errorf("expected empty after removing last ext, got %v", exts)
	}
}

func TestExtensions_RemoveNonExistent(t *testing.T) {
	cfg := &GlobalConfig{}
	// Should not panic
	cfg.RemoveExtension("nonexistent")
}

// Pre-existing configs from before the dns.enabled field was introduced have
// no `enabled:` key under `dns:`. LoadGlobal must preserve the `true` default
// for those users so an upgrade does not silently disable DNS.
func TestDNSEnabled_DefaultsTrueWhenKeyAbsent(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := []byte("dns:\n  tld: test\nphp:\n  default_version: 8.4\n")
	if err := os.WriteFile(GlobalConfigFile(), legacy, 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if !got.DNS.Enabled {
		t.Errorf("DNS.Enabled = false on legacy config without enabled key, want true")
	}
	if got.DNS.TLD != "test" {
		t.Errorf("DNS.TLD = %q, want %q", got.DNS.TLD, "test")
	}
}

func TestDNSEnabled_RoundTripsThroughYAML(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.DNS.Enabled = false
	cfg.DNS.TLD = "localhost"
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.DNS.Enabled {
		t.Errorf("DNS.Enabled = true, want false after roundtrip")
	}
	if got.DNS.TLD != "localhost" {
		t.Errorf("DNS.TLD = %q, want %q", got.DNS.TLD, "localhost")
	}
}

func TestMigrateStaleServiceImages_LeavesTrackLatestAlone(t *testing.T) {
	// Once postgres opted into track_latest, defaultConfig leaves its Image
	// empty so EnsureDefaultPresetQuadlet can resolve the actual newest tag
	// at install time. The stale-image migration must NOT rewrite to that
	// empty seed — doing so would land users in the fresh-install branch and
	// silently bump their data dir across major lines.
	cfg := defaultConfig()
	cfg.Services["postgres"] = ServiceConfig{
		Enabled: false,
		Image:   "postgres:16-alpine",
		Port:    5432,
	}
	migrateStaleServiceImages(cfg)
	if got := cfg.Services["postgres"].Image; got != "postgres:16-alpine" {
		t.Errorf("track_latest preset must keep saved image untouched, got %q", got)
	}
}

func TestMigrateStaleServiceImages_KeepsCustom(t *testing.T) {
	cfg := defaultConfig()
	cfg.Services["postgres"] = ServiceConfig{
		Enabled: true,
		Image:   "myorg/custom-postgres:latest",
		Port:    5432,
	}
	migrateStaleServiceImages(cfg)
	if got := cfg.Services["postgres"].Image; got != "myorg/custom-postgres:latest" {
		t.Errorf("custom postgres image was overwritten: got %q", got)
	}
}

// ── Workers.ExecMode ──────────────────────────────────────────────────────────

func TestWorkerExecMode_Defaults(t *testing.T) {
	cfg := defaultConfig()
	if got := cfg.WorkerExecMode(); got != WorkerExecModeExec {
		t.Errorf("default WorkerExecMode: got %q, want %q", got, WorkerExecModeExec)
	}
}

func TestWorkerExecMode_RespectsContainer(t *testing.T) {
	cfg := defaultConfig()
	cfg.Workers.ExecMode = WorkerExecModeContainer
	if got := cfg.WorkerExecMode(); got != WorkerExecModeContainer {
		t.Errorf("container override not respected: got %q", got)
	}
}

func TestWorkerExecMode_NormalizesUnknownValue(t *testing.T) {
	cfg := defaultConfig()
	cfg.Workers.ExecMode = "garbage"
	if got := cfg.WorkerExecMode(); got != WorkerExecModeExec {
		t.Errorf("unknown value should normalize to exec, got %q", got)
	}
}

func TestWorkerExecMode_RoundTripsThroughYAML(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.Workers.ExecMode = WorkerExecModeContainer
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	reloaded, err := LoadGlobal()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.WorkerExecMode(); got != WorkerExecModeContainer {
		t.Errorf("after round trip: got %q, want %q", got, WorkerExecModeContainer)
	}
}

// ── Notifications ─────────────────────────────────────────────────────────────

func TestNotifications_DefaultEnabled(t *testing.T) {
	cfg := &GlobalConfig{}
	if !cfg.IsNotificationsEnabled() {
		t.Error("zero-value config should report notifications enabled")
	}
}

func TestNotifications_Toggle(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.SetNotificationsEnabled(false)
	if cfg.IsNotificationsEnabled() {
		t.Error("after SetNotificationsEnabled(false), IsNotificationsEnabled should be false")
	}
	if !cfg.Notifications.Disabled {
		t.Error("Notifications.Disabled should be true when disabled")
	}
	cfg.SetNotificationsEnabled(true)
	if !cfg.IsNotificationsEnabled() {
		t.Error("after SetNotificationsEnabled(true), IsNotificationsEnabled should be true")
	}
	if cfg.Notifications.Disabled {
		t.Error("Notifications.Disabled should be false when enabled")
	}
}

func TestNotifications_DefaultsEnabledForLegacyConfig(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := []byte("php:\n  default_version: 8.4\n")
	if err := os.WriteFile(GlobalConfigFile(), legacy, 0644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if !got.IsNotificationsEnabled() {
		t.Error("legacy config without notifications key should default to enabled")
	}
}

func TestNotifications_RoundTripsThroughYAML(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.SetNotificationsEnabled(false)
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.IsNotificationsEnabled() {
		t.Error("notifications should remain disabled after YAML round trip")
	}
}

// ── Tray icon ─────────────────────────────────────────────────────────────────

func TestTrayIcon_DefaultThemeAdaptive(t *testing.T) {
	cfg := &GlobalConfig{}
	if cfg.IsHighContrastTrayIcon() {
		t.Error("zero-value config should report the theme-adaptive tray icon")
	}
}

func TestTrayIcon_Toggle(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.SetHighContrastTrayIcon(true)
	if !cfg.IsHighContrastTrayIcon() {
		t.Error("after SetHighContrastTrayIcon(true), IsHighContrastTrayIcon should be true")
	}
	cfg.SetHighContrastTrayIcon(false)
	if cfg.IsHighContrastTrayIcon() {
		t.Error("after SetHighContrastTrayIcon(false), IsHighContrastTrayIcon should be false")
	}
}

func TestTrayIcon_RoundTripsThroughYAML(t *testing.T) {
	setConfigDir(t)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.SetHighContrastTrayIcon(true)
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got, err := LoadGlobal()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.IsHighContrastTrayIcon() {
		t.Error("high-contrast tray icon should persist across a YAML round trip")
	}
}

func TestDNSManaged(t *testing.T) {
	var nilCfg *GlobalConfig
	if !nilCfg.DNSManaged() {
		t.Error("nil config should count as DNS-managed, matching the rest of the codebase")
	}
	enabled := &GlobalConfig{}
	enabled.DNS.Enabled = true
	if !enabled.DNSManaged() {
		t.Error("DNSManaged() = false with DNS.Enabled true, want true")
	}
	disabled := &GlobalConfig{}
	disabled.DNS.Enabled = false
	if disabled.DNSManaged() {
		t.Error("DNSManaged() = true with DNS.Enabled false, want false")
	}
}

// TestServiceConfig_HostPorts is the single source both the serviceops port guard
// and the host-proxy allocator consume, so it must capture the effective primary
// (the PublishedPort override when set, else the preset-default Port) and every
// ExtraPorts mapping form — including "ip:host:container", whose host side the old
// guard parser dropped.
func TestServiceConfig_HostPorts(t *testing.T) {
	svc := ServiceConfig{
		Port:          3306,
		PublishedPort: 3307,
		ExtraPorts:    []string{"8082:8081", "127.0.0.1:9090:9090", "7000", "6379/tcp"},
	}
	got := map[int]bool{}
	for _, p := range svc.HostPorts() {
		got[p] = true
	}
	for _, want := range []int{3307, 8082, 9090, 7000, 6379} {
		if !got[want] {
			t.Errorf("HostPorts missing %d; got %v", want, got)
		}
	}
	// Once an override moves the service off its default, the quadlet publishes
	// only the override, so the freed default must NOT stay reserved — otherwise
	// it can never be reassigned to another service.
	if got[3306] {
		t.Errorf("HostPorts still reserved the freed default 3306 after an override: %v", got)
	}
	// The container-side ports must never be reserved as host ports.
	if got[8081] {
		t.Errorf("HostPorts wrongly reserved container-side port 8081: %v", got)
	}
}

// TestServiceConfig_HostPorts_defaultWhenNoOverride reports the preset-default
// Port when no PublishedPort override is set.
func TestServiceConfig_HostPorts_defaultWhenNoOverride(t *testing.T) {
	svc := ServiceConfig{Port: 5432}
	got := svc.HostPorts()
	if len(got) != 1 || got[0] != 5432 {
		t.Errorf("HostPorts = %v, want [5432]", got)
	}
}

// ── FPMPorts ─────────────────────────────────────────────────────────────────

func TestFPMPortsFor_RoundTrip(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.PHP.FPMPorts = map[string][]string{"8.3": {"3000:3000", "5173:5173"}}
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	got := FPMPortsFor("8.3")
	if len(got) != 2 || got[0] != "3000:3000" || got[1] != "5173:5173" {
		t.Errorf("FPMPortsFor(8.3) = %v, want [3000:3000 5173:5173]", got)
	}
	if v := FPMPortsFor("8.4"); v != nil {
		t.Errorf("FPMPortsFor(8.4) = %v, want nil", v)
	}
}

// The version key carries a dot, so this pins that viper's "::" delimiter keeps
// "8.3" a single key rather than nesting it under an "8" map.
func TestFPMPortsFor_DottedVersionKeySurvivesLoad(t *testing.T) {
	setConfigDir(t)
	cfg, _ := LoadGlobal()
	cfg.PHP.FPMPorts = map[string][]string{"8.3": {"9000:9000"}}
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	reloaded, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if got := reloaded.PHP.FPMPorts["8.3"]; len(got) != 1 || got[0] != "9000:9000" {
		t.Errorf("reloaded FPMPorts[8.3] = %v, want [9000:9000]", got)
	}
}

// A version's FPM ports must be reserved so the service port-ownership guard
// never hands one of them to a service and collides at boot.
func TestReservedHostPorts_IncludesFPMPorts(t *testing.T) {
	setConfigDir(t)
	cfg, _ := LoadGlobal()
	cfg.PHP.FPMPorts = map[string][]string{"8.3": {"3000:3000"}}
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if !ReservedHostPorts()[3000] {
		t.Errorf("ReservedHostPorts must reserve an FPM port 3000; got %v", ReservedHostPorts())
	}
}

// A published-port override that moves a service off its preset default must free
// that default for reuse: the preset's own default port loop must not keep it
// reserved, matching HostPorts()'s freed-default contract.
func TestReservedHostPorts_FreesDefaultWhenPublishedOverrideMovesIt(t *testing.T) {
	setConfigDir(t)
	cfg, _ := LoadGlobal()
	svc := cfg.Services["mysql"]
	def := svc.Port
	if def == 0 {
		t.Skip("mysql preset has no default port in this build")
	}
	moved := def + 1000
	svc.PublishedPort = moved
	cfg.Services["mysql"] = svc
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	reserved := ReservedHostPorts()
	if reserved[def] {
		t.Errorf("moved-off default port %d must be freed, but it is still reserved", def)
	}
	if !reserved[moved] {
		t.Errorf("the new published port %d must be reserved; got %v", moved, reserved)
	}
}
