// Package dnsserver is lerd's built-in DNS server. It reads the dnsmasq-style
// config directory lerd has always written, so the format and every tool that
// rewrites lerd.conf stay the same.
package dnsserver

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const ttl = 60

// Addrs are the answers for every name under one domain.
type Addrs struct {
	V4 []net.IP
	V6 []net.IP
}

// Conf is the subset of dnsmasq's syntax this server acts on: port=, address=,
// server= (plain and per-domain) and log-queries.
type Conf struct {
	Port    int
	Domains map[string]Addrs
	// Upstreams answer every other name, as host:port. A resolver that routes
	// all queries here (systemd-resolved with ~.) relies on it.
	Upstreams []string
	// Routes sends names under a domain to that domain's own servers, from
	// server=/d/ip. An empty route refuses the names instead of forwarding, and
	// a defaultUpstreams entry stands for the default upstreams (server=/d/#).
	Routes map[string][]string
	// LogQueries prints every query to stderr, as dnsmasq's log-queries did.
	LogQueries bool
	// Unsupported holds the lines this server does not act on, so a directive
	// someone added by hand for dnsmasq is reported rather than lost.
	Unsupported []string
}

// ParseConf reads a lerd.conf. Directives it does not serve, and lines it
// cannot parse, are skipped so a newer config never stops an older server;
// the ones it does not recognise are listed in Unsupported.
func ParseConf(data []byte) (Conf, error) {
	c := Conf{Domains: map[string]Addrs{}, Routes: map[string][]string{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || line == "no-resolv":
		case strings.HasPrefix(line, "port="):
			if n, err := strconv.Atoi(strings.TrimPrefix(line, "port=")); err == nil {
				c.Port = n
			}
		case line == "log-queries":
			c.LogQueries = true
		case strings.HasPrefix(line, "server=/"):
			if !c.addRoute(strings.TrimPrefix(line, "server=/")) {
				c.Unsupported = append(c.Unsupported, line)
			}
		case strings.HasPrefix(line, "server="):
			if up, ok := parseUpstream(strings.TrimPrefix(line, "server=")); ok {
				c.Upstreams = append(c.Upstreams, up)
			} else {
				c.Unsupported = append(c.Unsupported, line)
			}
		case strings.HasPrefix(line, "address=/"):
			parts := strings.Split(strings.TrimPrefix(line, "address=/"), "/")
			if len(parts) != 2 {
				continue
			}
			ip := net.ParseIP(parts[1])
			if ip == nil {
				continue
			}
			domain := strings.ToLower(strings.Trim(parts[0], "."))
			a := c.Domains[domain]
			if ip.To4() != nil {
				a.V4 = append(a.V4, ip)
			} else {
				a.V6 = append(a.V6, ip)
			}
			c.Domains[domain] = a
		default:
			c.Unsupported = append(c.Unsupported, line)
		}
	}
	return c, sc.Err()
}

// defaultUpstreams marks a route that uses the default upstreams, dnsmasq's
// server=/d/# form.
const defaultUpstreams = "#"

// addRoute records server=/d1/d2/ip#port, given without its server=/ prefix,
// and reports whether it understood the target. Its domains are routed even
// when it did not, to nowhere, so a private name a hand-written rule kept off
// the default upstream is refused rather than leaked to it. The domain # is
// dnsmasq's "every domain", so it adds a default upstream instead.
func (c *Conf) addRoute(v string) bool {
	parts := strings.Split(v, "/")
	target := parts[len(parts)-1]
	up, ok := parseUpstream(target)
	if target == defaultUpstreams {
		up, ok = defaultUpstreams, true
	}
	for _, d := range parts[:len(parts)-1] {
		d = strings.ToLower(strings.Trim(d, "."))
		switch {
		case d == "":
		case d == "#":
			if ok && up != defaultUpstreams {
				c.Upstreams = append(c.Upstreams, up)
			}
		case ok:
			c.Routes[d] = append(c.Routes[d], up)
		default:
			if _, seen := c.Routes[d]; !seen {
				c.Routes[d] = nil
			}
		}
	}
	return ok || target == ""
}

// parseUpstream reads ip or ip#port.
func parseUpstream(v string) (string, bool) {
	host, port, hasPort := strings.Cut(v, "#")
	if net.ParseIP(host) == nil {
		return "", false
	}
	if !hasPort {
		port = "53"
	} else if _, err := strconv.Atoi(port); err != nil {
		return "", false
	}
	return net.JoinHostPort(host, port), true
}

// Server answers A and AAAA queries for the domains in a config directory,
// reloading it whenever a file in it changes.
type Server struct {
	dir string

	mu    sync.Mutex
	conf  Conf
	state string
	// lastGood is the upstream that answered last, asked first next time so a
	// dead server ahead of it in the list costs one timeout, not one per query.
	lastGood string

	srvs []*dns.Server
}

// New returns a Server backed by the config directory dir.
func New(dir string) *Server { return &Server{dir: dir} }

func (s *Server) config() Conf {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, state := confFiles(s.dir)
	if state == s.state {
		return s.conf
	}
	var data []byte
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(s.dir, f))
		if err != nil {
			return s.conf
		}
		data = append(append(data, b...), '\n')
	}
	c, err := ParseConf(data)
	if err != nil {
		return s.conf
	}
	s.conf, s.state = c, state
	for _, line := range c.Unsupported {
		fmt.Fprintf(os.Stderr, "lerd dns-serve: ignoring unsupported line in %s: %s\n", s.dir, line)
	}
	return s.conf
}

// confFiles lists the files dnsmasq's conf-dir would read, skipping dotfiles
// and editor backups, along with a fingerprint that changes when any of them do.
func confFiles(dir string) ([]string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ""
	}
	var files []string
	var state strings.Builder
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || strings.HasSuffix(n, "~") || (strings.HasPrefix(n, "#") && strings.HasSuffix(n, "#")) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, n)
		fmt.Fprintf(&state, "%s %d %d\n", n, info.ModTime().UnixNano(), info.Size())
	}
	return files, state.String()
}

// Port is the port the config asks for, or 0 when it names none.
func (s *Server) Port() int { return s.config().Port }

func (s *Server) handle(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	if len(req.Question) != 1 {
		resp.Rcode = dns.RcodeFormatError
		_ = w.WriteMsg(resp)
		return
	}
	q := req.Question[0]
	conf := s.config()
	if conf.LogQueries {
		fmt.Fprintf(os.Stderr, "query[%s] %s from %s\n", dns.TypeToString[q.Qtype], q.Name, w.RemoteAddr())
	}
	addrs, ok := lookupDomain(conf, q.Name)
	if !ok {
		upstreams := conf.Upstreams
		if route, routed := routeFor(conf, q.Name); routed {
			upstreams = expandRoute(route, conf.Upstreams)
		}
		_ = w.WriteMsg(s.forward(req, upstreams, w.LocalAddr().Network()))
		return
	}
	hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: ttl}
	switch q.Qtype {
	case dns.TypeA:
		hdr.Rrtype = dns.TypeA
		for _, ip := range addrs.V4 {
			resp.Answer = append(resp.Answer, &dns.A{Hdr: hdr, A: ip.To4()})
		}
	case dns.TypeAAAA:
		hdr.Rrtype = dns.TypeAAAA
		for _, ip := range addrs.V6 {
			resp.Answer = append(resp.Answer, &dns.AAAA{Hdr: hdr, AAAA: ip})
		}
	}
	_ = w.WriteMsg(resp)
}

// forwardTimeout is how long one upstream gets to answer. A seam for tests.
var forwardTimeout = 2 * time.Second

// forward relays req to the upstreams, the last one that answered first, and
// returns the first answer, SERVFAIL when none replies, or REFUSED when there
// are none to ask.
func (s *Server) forward(req *dns.Msg, upstreams []string, network string) *dns.Msg {
	fail := new(dns.Msg)
	fail.SetReply(req)
	if len(upstreams) == 0 {
		fail.Rcode = dns.RcodeRefused
		return fail
	}
	c := &dns.Client{Net: network, Timeout: forwardTimeout}
	for _, up := range s.upstreamOrder(upstreams) {
		if r, _, err := c.Exchange(req, up); err == nil {
			s.mu.Lock()
			s.lastGood = up
			s.mu.Unlock()
			return r
		}
	}
	fail.Rcode = dns.RcodeServerFailure
	return fail
}

// upstreamOrder moves the upstream that answered last to the front, provided
// it is still in the config.
func (s *Server) upstreamOrder(upstreams []string) []string {
	s.mu.Lock()
	last := s.lastGood
	s.mu.Unlock()
	if last == "" || upstreams[0] == last || !slices.Contains(upstreams, last) {
		return upstreams
	}
	ordered := []string{last}
	for _, up := range upstreams {
		if up != last {
			ordered = append(ordered, up)
		}
	}
	return ordered
}

// expandRoute replaces a route's defaultUpstreams entries with the defaults.
func expandRoute(route, defaults []string) []string {
	var out []string
	for _, up := range route {
		if up == defaultUpstreams {
			out = append(out, defaults...)
		} else {
			out = append(out, up)
		}
	}
	return out
}

// routeFor finds the most specific server=/d/ route covering name, d itself
// included, as dnsmasq matches it.
func routeFor(c Conf, name string) ([]string, bool) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	for name != "" {
		if r, ok := c.Routes[name]; ok {
			return r, true
		}
		_, name, _ = strings.Cut(name, ".")
	}
	return nil, false
}

// lookupDomain finds the configured domain name falls under. address=/.d/ip
// covers subdomains of d but not d itself, matching dnsmasq's behaviour here.
func lookupDomain(c Conf, name string) (Addrs, bool) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	for {
		i := strings.IndexByte(name, '.')
		if i < 0 {
			return Addrs{}, false
		}
		name = name[i+1:]
		if a, ok := c.Domains[name]; ok {
			return a, true
		}
	}
}

func (s *Server) serve(srv *dns.Server) error {
	s.mu.Lock()
	s.srvs = append(s.srvs, srv)
	s.mu.Unlock()
	return srv.ActivateAndServe()
}

// ServeUDP serves on pc until Shutdown. ready, when non-nil, runs once the
// socket is accepting queries.
func (s *Server) ServeUDP(pc net.PacketConn, ready func()) error {
	return s.serve(&dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(s.handle), NotifyStartedFunc: ready})
}

// ServeTCP serves on ln until Shutdown. DNS clients fall back to TCP for large
// answers, and lerd's own diagnostics probe the port over TCP.
func (s *Server) ServeTCP(ln net.Listener, ready func()) error {
	return s.serve(&dns.Server{Listener: ln, Handler: dns.HandlerFunc(s.handle), NotifyStartedFunc: ready})
}

// Shutdown stops every running Serve call.
func (s *Server) Shutdown() error {
	s.mu.Lock()
	srvs := s.srvs
	s.srvs = nil
	s.mu.Unlock()
	var first error
	for _, srv := range srvs {
		if err := srv.Shutdown(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
