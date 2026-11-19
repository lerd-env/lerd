package cli

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/goserver"
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
