package cli

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dnsserver"
)

// NewDNSServeCmd returns the hidden `lerd dns-serve` command that the lerd-dns
// service runs. It serves the lerd.conf directory, reloading it whenever the
// watcher or lan:expose rewrites it.
func NewDNSServeCmd() *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:    "dns-serve",
		Short:  "Built-in DNS server for the lerd TLD (internal)",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDNSServe(config.DnsmasqDir(), listen)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1", "address to listen on; the port comes from lerd.conf")
	return cmd
}

func runDNSServe(confDir, host string) error {
	srv := dnsserver.New(confDir)
	port := srv.Port()
	if port == 0 {
		return fmt.Errorf("no port= in %s; run `lerd install` first", confDir)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s (udp): %w", addr, err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("listening on %s (tcp): %w", addr, err)
	}

	errCh := make(chan error, 2)
	go func() { errCh <- srv.ServeUDP(pc, nil) }()
	go func() { errCh <- srv.ServeTCP(ln, nil) }()
	fmt.Fprintf(os.Stderr, "lerd dns-serve: answering on %s (UDP+TCP)\n", addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		return srv.Shutdown()
	}
}
