// Package tools resolves the pinned versions and download URLs of the host
// tools lerd installs (composer, fnm, mkcert). tools.yaml is the source of
// truth: embedded at build time as the offline fallback, and fetched from
// GitHub before use so a bad pin can be fixed without a binary release.
package tools

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/origin"
	"gopkg.in/yaml.v3"
)

//go:embed tools.yaml
var embedded []byte

// fetchTimeout bounds the best-effort manifest fetch; past it the embedded
// pins stand, so an offline install is delayed by at most this long.
// cacheTTL is how long a fetched (or failed) result is reused from disk, so
// status paths don't hit the network, or stall offline, more than daily.
var (
	fetchTimeout = 5 * time.Second
	cacheTTL     = 24 * time.Hour
)

// Tool is one pinned download: a version and an HTTPS URL template in which
// {version} and {asset} expand. Assets maps GOOS/GOARCH to the release asset
// name, which may itself contain {version}. Digests maps the same GOOS/GOARCH
// key to the asset's sha256, and is optional so a manifest published without one
// still installs on binaries that predate the field.
type Tool struct {
	Version string            `yaml:"version"`
	URL     string            `yaml:"url"`
	Assets  map[string]string `yaml:"assets"`
	Digests map[string]string `yaml:"digests,omitempty"`
}

// Manifest is the parsed tools.yaml.
type Manifest struct {
	Tools map[string]Tool `yaml:"tools"`
}

var (
	versionRe = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,3}$`)
	sha256Re  = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
)

// downloadHosts is where a published manifest may point. The manifest is fetched
// from a branch at runtime rather than shipped in the release, so it reaches
// every install without review; without this, editing one file would redirect
// what every host downloads and marks executable. Extendable through
// LERD_TOOLS_HOSTS for the same reason LERD_TOOLS_URL exists.
var downloadHosts = []string{
	"getcomposer.org",
	"github.com",
	"objects.githubusercontent.com",
	"release-assets.githubusercontent.com",
	"raw.githubusercontent.com",
}

// allowedDownloadHost reports whether an https URL points somewhere a tool may
// be fetched from. A bare suffix match would let evil-github.com through, so the
// host is compared whole.
func allowedDownloadHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range append(downloadHosts, origin.ExtraToolHosts()...) {
		if host == allowed {
			return true
		}
	}
	return false
}

// valid guards published values before they reach a download URL: a plain
// version string, an https template pointing at an allowed host, and, where the
// manifest gives one, a well-formed sha256 per asset.
func (t Tool) valid() bool {
	if !versionRe.MatchString(t.Version) || !allowedDownloadHost(t.URL) {
		return false
	}
	for _, d := range t.Digests {
		if !sha256Re.MatchString(d) {
			return false
		}
	}
	return true
}

// Digest returns the published sha256 for an asset, empty when the manifest
// pins none for this platform.
func (m *Manifest) Digest(name, goos, goarch string) string {
	return m.Tools[name].Digests[goos+"/"+goarch]
}

// Load returns the pinned tool manifest: the embedded copy, overlaid with
// every valid entry of the published tools.yaml when it is reachable.
func Load(ctx context.Context) *Manifest {
	m := embeddedManifest()
	for name, t := range publishedTools(ctx) {
		if t.valid() {
			m.Tools[name] = t
		}
	}
	return m
}

// Refresh reloads the manifest with the disk cache bypassed, for the manual
// "check for updates" path. The 24h cache is what keeps a passive status
// rebuild off the network, so a newly published pin is otherwise invisible for
// up to a day; this is the way out of that window. A fetch that cannot reach
// the endpoint leaves the cached pins in place rather than reverting to the
// embedded ones, so being offline never reads as "nothing to update".
func Refresh(ctx context.Context) *Manifest {
	if fetched, data := fetchPublished(ctx); fetched != nil {
		writeCache(manifestCachePath(), data)
	}
	return Load(ctx)
}

func manifestCachePath() string {
	return filepath.Join(config.DataDir(), "tools-manifest.yaml")
}

// publishedTools returns the published pins, preferring a fresh disk cache
// over the network. A failed fetch re-stamps the cache mtime so a dead
// endpoint is retried at most once per cacheTTL, with stale data (or the
// embedded pins, via an empty marker) standing in until it answers again.
func publishedTools(ctx context.Context) map[string]Tool {
	path := manifestCachePath()
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < cacheTTL {
		data, _ := os.ReadFile(path)
		return parseTools(data)
	}
	if fetched, data := fetchPublished(ctx); fetched != nil {
		writeCache(path, data)
		return fetched
	}
	data, err := os.ReadFile(path)
	writeCache(path, data)
	if err != nil {
		return nil
	}
	return parseTools(data)
}

func writeCache(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	os.WriteFile(path, data, 0o644) //nolint:errcheck
}

func parseTools(data []byte) map[string]Tool {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m.Tools
}

func embeddedManifest() *Manifest {
	var m Manifest
	if err := yaml.Unmarshal(embedded, &m); err != nil {
		m.Tools = map[string]Tool{}
	}
	return &m
}

// fetchPublished is best-effort: any network, status or parse problem returns
// nils and the caller falls back to cached or embedded pins.
func fetchPublished(ctx context.Context) (map[string]Tool, []byte) {
	client := &http.Client{Timeout: fetchTimeout}
	for _, url := range origin.ToolsManifestURLs() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if parsed := parseTools(data); parsed != nil {
			return parsed, data
		}
	}
	return nil, nil
}

// Names lists the managed tools in display order.
func Names() []string { return []string{"composer", "fnm", "mkcert"} }

func binPath(name string) string {
	bin := name
	if name == "composer" {
		bin = "composer.phar"
	}
	return filepath.Join(config.BinDir(), bin)
}

// WriteStamp records the installed version of a tool in its sidecar, so
// status can report it without executing anything.
func WriteStamp(name, version string) error {
	return os.WriteFile(binPath(name)+".version", []byte(version+"\n"), 0o644)
}

// InstalledVersion reports the version of an installed tool: the stamp
// sidecar while it is trustworthy (not older than the binary, which would
// mean something else replaced it, e.g. composer self-update), else a
// version probe. Empty when the binary is missing or undeterminable.
//
// A probed answer is stamped on the way out. The stamp is otherwise only
// written when lerd downloads a tool, and composer is only downloaded when it
// is absent, so an install that already had composer.phar before stamping
// existed never got one. With composer unprobeable by execution, it read as
// unknown forever and, because the update check needs a known version, was
// never offered an update either.
func InstalledVersion(name string) string {
	path := binPath(name)
	binInfo, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if stampInfo, err := os.Stat(path + ".version"); err == nil &&
		!binInfo.ModTime().After(stampInfo.ModTime().Add(time.Minute)) {
		if b, err := os.ReadFile(path + ".version"); err == nil {
			if v := strings.TrimSpace(string(b)); versionRe.MatchString(v) {
				return v
			}
		}
	}
	v := probeVersion(name, path)
	if v != "" {
		_ = WriteStamp(name, v)
	}
	return v
}

// probeOutput runs a binary's version flag, a seam for tests.
var probeOutput = func(path string, args ...string) ([]byte, error) {
	return exec.Command(path, args...).Output()
}

// composerVersionRe matches the version constant composer compiles into its
// phar. Anchored on the declaration: the archive also carries a version string
// for every bundled dependency, and a looser match would report one of those.
var composerVersionRe = regexp.MustCompile(`const VERSION = '([^']+)'`)

// composerPharMaxScan bounds the read. A composer phar is a few megabytes; well
// past this it is not one, and reading it would not be worth the memory.
const composerPharMaxScan = 64 << 20

// composerVersion reads composer's version out of the phar without running it.
// Executing it would need a PHP runtime lerd only has in containers, so the
// bytes are the only source available on the host.
func composerVersion(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.Size() > composerPharMaxScan {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := composerVersionRe.FindSubmatch(body)
	if m == nil {
		return ""
	}
	// A build from source carries something like 1.0.0+no-version-set, which is
	// not a release version. Reporting nothing beats reporting that.
	if v := strings.TrimSpace(string(m[1])); versionRe.MatchString(v) {
		return v
	}
	return ""
}

// probeVersion asks the binary itself, except composer, which is a phar and is
// read rather than run.
func probeVersion(name, path string) string {
	var arg string
	switch name {
	case "fnm":
		arg = "--version"
	case "mkcert":
		arg = "-version"
	case "composer":
		return composerVersion(path)
	default:
		return ""
	}
	out, err := probeOutput(path, arg)
	if err != nil {
		return ""
	}
	for _, f := range strings.Fields(string(out)) {
		if versionRe.MatchString(f) {
			return f
		}
	}
	return ""
}

// ToolStatus is one row of the tools report shared by the CLI, UI and MCP.
type ToolStatus struct {
	Name            string `json:"name"`
	Installed       string `json:"installed,omitempty"`
	Pinned          string `json:"pinned"`
	Present         bool   `json:"present"`
	UpdateAvailable bool   `json:"update_available"`
}

// StatusAll compares each installed tool with its pin.
func StatusAll(ctx context.Context) []ToolStatus {
	m := Load(ctx)
	out := make([]ToolStatus, 0, len(Names()))
	for _, name := range Names() {
		pin := m.Tools[name].Version
		installed := InstalledVersion(name)
		_, err := os.Stat(binPath(name))
		present := err == nil
		out = append(out, ToolStatus{
			Name: name, Installed: installed, Pinned: pin, Present: present,
			UpdateAvailable: present && installed != "" && pin != "" && !sameVersion(installed, pin),
		})
	}
	return out
}

func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// URL resolves the download URL for the named tool on goos/goarch.
func (m *Manifest) URL(name, goos, goarch string) (string, error) {
	t, ok := m.Tools[name]
	if !ok {
		return "", fmt.Errorf("no pinned version for %s", name)
	}
	asset := ""
	if len(t.Assets) > 0 {
		asset, ok = t.Assets[goos+"/"+goarch]
		if !ok {
			return "", fmt.Errorf("%s has no release asset for %s/%s", name, goos, goarch)
		}
		asset = strings.ReplaceAll(asset, "{version}", t.Version)
	}
	url := strings.NewReplacer("{version}", t.Version, "{asset}", asset).Replace(t.URL)
	if strings.ContainsAny(url, "{}") {
		return "", fmt.Errorf("unresolved placeholder in %s URL %q", name, url)
	}
	return url, nil
}
