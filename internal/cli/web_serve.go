package cli

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/goserver"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
)

// NewWebServeCmd runs lerd's own web server in the foreground. It is what the
// web server container runs when lerd serves sites without nginx.
func NewWebServeCmd() *cobra.Command {
	var listen, listenTLS string
	cmd := &cobra.Command{
		Use:    "web-serve",
		Short:  "Serve sites with lerd's built-in web server (runs inside its container)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h := &goserver.Handler{Sites: registeredSites, Upstream: fpmAddr, Proxy: goserver.SiteTarget, Services: serviceRoutes, WorkerHost: workerHost}
			h.WakeHold, h.WakeTransport = lerdUI()
			h.ProfilerUpstream = profilerUpstream
			served := &goserver.AccessFeed{Next: h, Target: config.AccessLogTarget()}
			tlsSrv := &http.Server{Addr: listenTLS, Handler: served, TLSConfig: &tls.Config{GetCertificate: h.GetCertificate}}
			errs := make(chan error, 2)
			go func() { errs <- http.ListenAndServe(listen, served) }()
			go func() { errs <- tlsSrv.ListenAndServeTLS("", "") }()
			return <-errs
		},
	}
	cmd.Flags().StringVar(&listen, "listen", ":80", "address to serve plain HTTP on")
	cmd.Flags().StringVar(&listenTLS, "listen-tls", ":443", "address to serve HTTPS on")
	return cmd
}

func registeredSites() []config.Site {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	return reg.Sites
}

// workerHost is where a site's worker listens: the host for one that runs
// there, else the PHP container the site's own requests go to.
func workerHost(site config.Site, onHost bool) string {
	if onHost {
		return podman.HostProxyUpstream()
	}
	host, _, _ := net.SplitHostPort(fpmAddr(site))
	return host
}

func fpmAddr(site config.Site) string {
	version := site.PHPVersion
	if version == "" {
		if cfg, err := config.LoadGlobal(); err == nil {
			version = cfg.PHP.DefaultVersion
		}
	}
	host, port := podman.FPMUpstream(site, version)
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func serviceRoutes() []goserver.ServiceRoute {
	tld := config.EffectiveTLD()
	var routes []goserver.ServiceRoute
	for _, r := range serviceops.DomainRoutes() {
		routes = append(routes, goserver.ServiceRoute{
			Domain:   r.Domain,
			Upstream: net.JoinHostPort("lerd-"+r.Service, strconv.Itoa(r.Port)),
			CORS:     r.CORS,
			TLD:      tld,
		})
	}
	return routes
}

// lerdUI is lerd-ui as the web server container reaches it, the same way the
// nginx waking vhost does: its unix socket on Linux, the host on macOS.
func lerdUI() (*url.URL, http.RoundTripper) {
	if platform.Current.UsesMachineVM {
		return &url.URL{Scheme: "http", Host: "host.containers.internal:7073"}, nil
	}
	sock := config.UISocketPath()
	return &url.URL{Scheme: "http", Host: "lerd-ui"}, &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}
}

// profilerUpstream is the PHP-FPM and bridge script the SPX dashboard runs on,
// chosen as the nginx profiler vhost chooses them: the host's PHP-FPM in
// native mode, else the default version's container.
func profilerUpstream() (string, string) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		cfg = &config.GlobalConfig{}
	}
	version := cfg.PHP.DefaultVersion
	if cfg.PHPRuntimeMode() == config.PHPRuntimeNative {
		if port, err := nativephp.PortFor(version); err == nil {
			return net.JoinHostPort(podman.HostGateway, strconv.Itoa(port)), config.DumpsBridgeFile()
		}
	}
	host, port := podman.FPMUpstream(config.Site{PHPVersion: version}, version)
	return net.JoinHostPort(host, strconv.Itoa(port)), "/usr/local/etc/lerd/dump-bridge.php"
}
