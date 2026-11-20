package webserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/goserver"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/webcustom"
)

// GoImage is the image lerd's own web server runs in: any small Linux will do,
// since the static lerd binary is mounted into it.
const GoImage = "docker.io/library/alpine:latest"

// goBackend is lerd's own web server. It reads the site registry, the service
// domains and its customisation files on every request, so nothing is written
// per site and nothing has to be reloaded.
type goBackend struct{}

func (goBackend) ApplySite(config.Site) error                        { return nil }
func (goBackend) RemoveSite(string) error                            { return nil }
func (goBackend) Reload() error                                      { return nil }
func (goBackend) ReloadWithRetry(time.Duration) error                { return nil }
func (goBackend) ReloadAndSettle() error                             { return nil }
func (goBackend) ServesVhostWrittenAt(time.Time) bool                { return true }
func (goBackend) ApplyPausedSite(config.Site) error                  { return nil }
func (goBackend) ApplyWakingSite(config.Site) error                  { return nil }
func (goBackend) Drift(config.Site) (DriftReport, error)             { return DriftReport{}, nil }
func (goBackend) RegenerateSite(config.Site) error                   { return nil }
func (goBackend) Repair() []Repair                                   { return nil }
func (goBackend) EnsureConfig() error                                { return nil }
func (goBackend) EnsureDefaultSite() error                           { return nil }
func (goBackend) EnsureDashboard() error                             { return nil }
func (goBackend) SyncDashboard() (bool, error)                       { return false, nil }
func (goBackend) EnsureProfiler() error                              { return nil }
func (goBackend) ApplyService(string, string, int, bool, bool) error { return nil }
func (goBackend) ApplyWorktree(string, string, string, string, string, string, bool) error {
	return nil
}
func (goBackend) ApplyHostProxyWorktree(string, string, string, int, bool, bool) error { return nil }
func (goBackend) ApplyPausedWorktree(string, string, string, bool) error               { return nil }

// Test checks every customisation file the way the server will read it.
func (goBackend) Test() (string, error) {
	files, _ := filepath.Glob(filepath.Join(webcustom.Dir(), "custom.d", "*.yaml"))
	files = append([]string{webcustom.GlobalPath()}, files...)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := webcustom.Validate(f, data); err != nil {
			return err.Error(), err
		}
	}
	return "", nil
}

func (goBackend) ServedProfilerState() (bool, bool) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return false, false
	}
	return cfg.IsProfilerEnabled(), true
}

func (goBackend) SiteWaking(domain string) bool {
	site, err := config.FindSiteByDomain(domain)
	return err == nil && site != nil && goserver.Waking(*site)
}

// RewriteUnit writes the container that runs `lerd web-serve` from this very
// binary under the web server's unit name, on lerd's own data and config.
func (b goBackend) RewriteUnit() (bool, error) {
	content, err := b.UnitContent(podman.ExtraVolumePaths())
	if err != nil {
		return false, err
	}
	return podman.WriteQuadletDiff(podman.WebServerUnit(), content)
}

func (goBackend) UnitContent(extraPaths []string) (string, error) {
	content, err := podman.GetQuadletTemplate("lerd-goweb.container")
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the lerd binary to run: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	extra := strings.Join([]string{
		"Volume=" + exe + ":/usr/local/bin/lerd:ro",
		"Volume=" + config.RunDir() + ":" + config.RunDir() + ":rw",
		"Environment=HOME=%h",
		"Environment=XDG_DATA_HOME=" + filepath.Dir(config.DataDir()),
		"Environment=XDG_CONFIG_HOME=" + filepath.Dir(config.ConfigDir()),
	}, "\n")
	content = strings.Replace(content, "PodmanArgs=", extra+"\nPodmanArgs=", 1)
	httpPort, httpsPort := config.NginxPorts()
	content = podman.ApplyNginxPorts(content, httpPort, httpsPort)
	return podman.InjectExtraVolumes(content, extraPaths), nil
}
