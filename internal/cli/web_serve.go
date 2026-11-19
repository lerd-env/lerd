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
			h := &goserver.Handler{Sites: registeredSites, Upstream: fpmAddr, Proxy: goserver.SiteTarget}
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
