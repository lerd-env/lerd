package nginx

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/pfredirect"
)

// hostFiles are what the nginx image carries beside nginx.conf and the host's
// nginx reads from lerd's own directory instead, taken from Homebrew's nginx.
//
//go:embed hostfiles/mime.types hostfiles/fastcgi_params
var hostFiles embed.FS

// ensureHostFiles writes them beside nginx.conf, where `include fastcgi_params`
// resolves for an nginx started with lerd's config.
func ensureHostFiles() error {
	for _, name := range []string{"mime.types", "fastcgi_params"} {
		b, err := hostFiles.ReadFile("hostfiles/" + name)
		if err != nil {
			return err
		}
		if err := writeConf(filepath.Join(config.NginxDir(), name), b); err != nil {
			return err
		}
	}
	return nil
}

// The port forward relays IPv4 only, so the IPv6 listens have nothing to bind.
var ipv6Listen = regexp.MustCompile(`(?m)^[ \t]*listen \[::\]:(80|443)\b[^;\n]*;\n`)

var containerListen = regexp.MustCompile(`\blisten (80|443)\b`)

// hostForm turns a config rendered for the lerd-nginx container into the one the
// host's nginx reads: the folders mounted at /etc/nginx, loopback ports in place
// of 80 and 443, and loopback for the host gateway. Applying it twice is a no-op.
func hostForm(conf []byte) []byte {
	httpPort, httpsPort := config.NginxPorts()
	conf = ipv6Listen.ReplaceAll(conf, nil)
	conf = containerListen.ReplaceAllFunc(conf, func(m []byte) []byte {
		port := httpsPort
		if bytes.HasSuffix(m, []byte("80")) {
			port = httpPort
		}
		return fmt.Appendf(nil, "listen 127.0.0.1:%d", hostListenPort(port))
	})
	for _, p := range [][2]string{
		{"/etc/nginx/certs", filepath.Join(config.CertsDir(), "sites")},
		{"/etc/nginx/custom.d", config.NginxCustomD()},
		{"/etc/nginx/conf.d", config.NginxConfD()},
		{"/etc/nginx/http.d", config.NginxHttpD()},
		{"/etc/nginx/mime.types", filepath.Join(config.NginxDir(), "mime.types")},
		{hostGateway, "127.0.0.1"},
		{"user root;\n", ""},
		{"pid /tmp/nginx.pid;", "pid " + filepath.Join(config.RunDir(), "nginx.pid") + ";"},
		{"$server_port", "$lerd_public_port"},
	} {
		conf = bytes.ReplaceAll(conf, []byte(p[0]), []byte(p[1]))
	}
	return conf
}

// hostListenPort is where the host's nginx listens for a public port: the
// loopback one the forward relays to below 1024, the port itself otherwise.
func hostListenPort(public int) int {
	for _, p := range pfredirect.Ports() {
		if p == public {
			return pfredirect.Target(p)
		}
	}
	return public
}

// asHost is hostForm when nginx runs on the host, and conf untouched otherwise.
func asHost(conf []byte) []byte {
	if !OnHost() {
		return conf
	}
	return hostForm(conf)
}

// writeConf writes one of lerd's generated nginx configs in the form the nginx
// serving it reads. A user's own overrides are written as they are, elsewhere.
func writeConf(path string, conf []byte) error {
	return WriteFileAtomic(path, asHost(conf), 0644)
}

// publicPortMap gives $lerd_public_port, the port a client reached: nginx's own
// is the loopback one behind the forward. ${server_port} is the same variable,
// spelt so hostForm leaves it alone.
func publicPortMap() string {
	httpPort, httpsPort := config.NginxPorts()
	return fmt.Sprintf("map ${server_port} $lerd_public_port {\n    default ${server_port};\n    %d %d;\n    %d %d;\n}\n",
		hostListenPort(httpPort), httpPort, hostListenPort(httpsPort), httpsPort)
}
