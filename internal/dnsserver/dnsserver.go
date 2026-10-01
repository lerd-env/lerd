// Package dnsserver is lerd's built-in DNS answerer for hosts with no usable
// dnsmasq (Windows). It reads the same lerd.conf the dnsmasq container would,
// so the rest of lerd keeps writing one config and this serves it.
package dnsserver

import (
	"bufio"
	"bytes"
	"net"
	"os"
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

// Conf is the subset of dnsmasq's syntax lerd writes: port= and address=/.d/ip.
type Conf struct {
	Port    int
	Domains map[string]Addrs
}

// ParseConf reads a lerd.conf. Directives it does not serve, and lines it
// cannot parse, are skipped so a newer config never stops an older server.
func ParseConf(data []byte) (Conf, error) {
	c := Conf{Domains: map[string]Addrs{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "port="):
			if n, err := strconv.Atoi(strings.TrimPrefix(line, "port=")); err == nil {
				c.Port = n
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
		}
	}
	return c, sc.Err()
}

// Server answers A and AAAA queries for the domains in a config file, reloading
// it whenever its modification time changes.
type Server struct {
	path string

	mu      sync.Mutex
	conf    Conf
	modTime time.Time

	srvs []*dns.Server
}

// New returns a Server backed by the config file at path.
func New(path string) *Server { return &Server{path: path} }

func (s *Server) config() Conf {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, err := os.Stat(s.path); err == nil && !st.ModTime().Equal(s.modTime) {
		if data, err := os.ReadFile(s.path); err == nil {
			if c, err := ParseConf(data); err == nil {
				s.conf, s.modTime = c, st.ModTime()
			}
		}
	}
	return s.conf
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
	addrs, ok := lookupDomain(s.config(), q.Name)
	if !ok {
		resp.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(resp)
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
