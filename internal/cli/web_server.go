package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/geodro/lerd/internal/webserver"
)

// NewWebServerCmd shows or switches the web server that serves the sites.
func NewWebServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "web-server [nginx|go]",
		Short: "Show or switch the web server serving your sites (nginx or lerd's own)",
		Long: `Show which web server serves your sites, or switch between nginx and lerd's own
web server written in Go. A switch that does not come up healthy is rolled back.
show, edit and reset manage the settings lerd's own web server reads, per site
or with --global for every site.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cfg, err := config.LoadGlobal()
				if err != nil {
					return err
				}
				kind, known := cfg.WebServerKind()
				if !known {
					fmt.Fprintf(cmd.OutOrStdout(), "config names web server %q, which this lerd does not know; serving with nginx\n", cfg.WebServer)
				}
				fmt.Fprintln(cmd.OutOrStdout(), kind)
				return nil
			}
			return SwitchWebServer(args[0], cmd.OutOrStdout())
		},
	}
	cmd.AddCommand(newWebServerShowCmd(), newWebServerEditCmd(), newWebServerResetCmd())
	return cmd
}

// The steps a switch takes outside lerd's own files, swapped in tests.
var (
	webServerRestart = func() error {
		if err := podman.DaemonReloadFn(); err != nil {
			return err
		}
		return podman.RestartUnit(podman.WebServerUnit())
	}
	webServerHealthy = waitWebServerAnswers
	webServerPull    = podman.PullImageIfMissing
	webServerOnVM    = func() bool { return platform.Current.UsesMachineVM }
	// webServerStop stops and removes the container of the server being left.
	webServerStop = func(unit string) {
		_ = podman.StopUnit(unit)
		podman.RemoveContainer(unit)
	}
)

// SwitchWebServer moves the sites to another web server and rolls back to the
// previous one when the new one does not answer.
func SwitchWebServer(to string, out io.Writer) error {
	if err := config.ValidateWebServer(to); err != nil {
		return err
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	from, _ := cfg.WebServerKind()
	if from == to {
		fmt.Fprintf(out, "already serving with %s\n", to)
		return nil
	}
	if to == config.WebServerGo {
		if webServerOnVM() {
			return errors.New("lerd's own web server is Linux only for now: on macOS it would run inside the podman machine, where this binary cannot")
		}
		if files := nginxOverrides(); len(files) > 0 {
			return fmt.Errorf("these nginx overrides cannot follow the sites to lerd's own web server: %s; move what they do into %s and remove them, then switch again",
				strings.Join(files, ", "), filepath.Join(config.DataDir(), "web"))
		}
		if err := webServerPull(webserver.GoImage); err != nil {
			return fmt.Errorf("pulling %s: %w", webserver.GoImage, err)
		}
	}
	fmt.Fprintf(out, "switching from %s to %s\n", from, to)
	if err := applyWebServer(to); err != nil {
		return err
	}
	healthErr := webServerHealthy()
	if healthErr == nil {
		fmt.Fprintf(out, "serving with %s\n", to)
		return nil
	}
	fmt.Fprintf(out, "%s did not answer (%v), going back to %s\n", to, healthErr, from)
	if err := applyWebServer(from); err != nil {
		return fmt.Errorf("%s did not answer (%v), and going back to %s failed too: %w", to, healthErr, from, err)
	}
	return fmt.Errorf("%s did not answer, sites are back on %s: %w", to, from, healthErr)
}

// applyWebServer records the choice, writes what that server reads, and
// restarts the web server unit onto it.
func applyWebServer(kind string) error {
	// The other server is stopped and its unit removed, so it cannot come back
	// at the next login or be mistaken for the one serving.
	for _, other := range []string{config.WebServerNginx, config.WebServerGo} {
		if other == kind {
			continue
		}
		unit := podman.WebServerUnitFor(other)
		webServerStop(unit)
		if err := podman.RemoveQuadlet(unit); err != nil {
			return fmt.Errorf("removing %s: %w", unit, err)
		}
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	cfg.WebServer = kind
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	if kind == config.WebServerNginx {
		if err := rebuildNginxConfig(); err != nil {
			return err
		}
	}
	if _, err := webserver.RewriteUnit(); err != nil {
		return err
	}
	return webServerRestart()
}

// rebuildNginxConfig writes every vhost nginx needs for the registry as it is
// now, since sites linked while lerd's own server ran have none.
func rebuildNginxConfig() error {
	for _, ensure := range []func() error{webserver.EnsureConfig, webserver.EnsureDefaultSite, webserver.EnsureDashboard, webserver.EnsureProfiler} {
		if err := ensure(); err != nil {
			return err
		}
	}
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}
	for i := range reg.Sites {
		s := reg.Sites[i]
		switch {
		case s.Ignored:
		case s.Paused:
			err = webserver.ApplyPausedSite(s)
		case siteops.ServedByFPM(s):
			err = siteops.RegenerateFPMSiteVhosts(s)
		default:
			err = siteops.RegenerateSiteVhost(&s, s.PrimaryDomain())
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] vhost for %s: %v\n", s.Name, err)
		}
	}
	rewakeSleepingSites(reg.Sites)
	// Written without the reload ApplyServiceDomains asks for: the server still
	// running is the one being replaced, and the restart after loads them.
	for _, r := range serviceops.DomainRoutes() {
		if err := webserver.ApplyService(r.Domain, "lerd-"+r.Service, r.Port, true, r.CORS); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] service domain %s: %v\n", r.Domain, err)
		}
	}
	return nil
}

// nginxOverrides lists the raw nginx config a user wrote, per site and global,
// which lerd's own web server cannot read.
func nginxOverrides() []string {
	var files []string
	entries, _ := os.ReadDir(config.NginxCustomD())
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), ".conf") {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(config.NginxCustomD(), e.Name())); err == nil && strings.TrimSpace(string(data)) != "" {
			files = append(files, filepath.Join(config.NginxCustomD(), e.Name()))
		}
	}
	if data, err := os.ReadFile(config.NginxHttpUserConf()); err == nil && strings.TrimSpace(string(data)) != "" {
		files = append(files, config.NginxHttpUserConf())
	}
	sort.Strings(files)
	return files
}

// waitWebServerAnswers waits for any HTTP answer from the dashboard host on
// lerd's HTTP port: the server is up even if lerd-ui behind it is not.
func waitWebServerAnswers() error {
	httpPort, _ := config.NginxPorts()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(httpPort))
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		req.Host = "lerd.localhost"
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		lastErr = err
	}
	return lastErr
}
