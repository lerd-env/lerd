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
// name, which may itself contain {version}.
type Tool struct {
	Version string            `yaml:"version"`
	URL     string            `yaml:"url"`
	Assets  map[string]string `yaml:"assets"`
}

// Manifest is the parsed tools.yaml.
type Manifest struct {
	Tools map[string]Tool `yaml:"tools"`
}

var versionRe = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,3}$`)

// valid guards published values before they reach a download URL: anything
// but a plain version string or an HTTPS template is rejected.
func (t Tool) valid() bool {
	return versionRe.MatchString(t.Version) && strings.HasPrefix(t.URL, "https://")
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
	return probeVersion(name, path)
}

// probeOutput runs a binary's version flag, a seam for tests.
var probeOutput = func(path string, args ...string) ([]byte, error) {
	return exec.Command(path, args...).Output()
}

// probeVersion asks the binary itself; composer is skipped because running a
// phar needs a PHP runtime lerd only has in containers.
func probeVersion(name, path string) string {
	var arg string
	switch name {
	case "fnm":
		arg = "--version"
	case "mkcert":
		arg = "-version"
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
