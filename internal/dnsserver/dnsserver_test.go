package dnsserver

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"
)

const sampleConf = `# Lerd DNS configuration
port=5300
no-resolv
server=1.1.1.1
address=/.test/127.0.0.1
address=/.test/::1
`

func TestParseConf(t *testing.T) {
	c, err := ParseConf([]byte(sampleConf))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 5300 {
		t.Errorf("port = %d", c.Port)
	}
	if got := c.Domains["test"]; len(got.V4) != 1 || got.V4[0].String() != "127.0.0.1" || len(got.V6) != 1 || got.V6[0].String() != "::1" {
		t.Errorf("test domain = %+v", got)
	}
}

func TestParseConfIgnoresUnknownAndBadLines(t *testing.T) {
	c, err := ParseConf([]byte("cache-size=0\naddress=/.x/not-an-ip\naddress=/.ok/10.0.0.5\nport=abc\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Domains["x"]; ok {
		t.Error("an unparseable address must be dropped")
	}
	if got := c.Domains["ok"].V4; len(got) != 1 || got[0].String() != "10.0.0.5" {
		t.Errorf("ok domain = %v", got)
	}
	if c.Port != 0 {
		t.Errorf("unparseable port should read as unset, got %d", c.Port)
	}
}

func startTestServer(t *testing.T, conf string) (addr string, path string, stop func()) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "lerd.conf")
	if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(path)
	done := make(chan struct{})
	started := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.ServeUDP(pc, func() { close(started) })
	}()
	<-started
	return pc.LocalAddr().String(), path, func() { _ = srv.Shutdown(); <-done }
}

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	c := &dns.Client{Timeout: 2 * time.Second}
	r, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAnswersConfiguredDomain(t *testing.T) {
	addr, _, stop := startTestServer(t, sampleConf)
	defer stop()

	r := query(t, addr, "shop.test", dns.TypeA)
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 {
		t.Fatalf("A: rcode=%d answers=%v", r.Rcode, r.Answer)
	}
	if a := r.Answer[0].(*dns.A).A.String(); a != "127.0.0.1" {
		t.Errorf("A = %s", a)
	}
	r = query(t, addr, "a.b.shop.test", dns.TypeAAAA)
	if len(r.Answer) != 1 || r.Answer[0].(*dns.AAAA).AAAA.String() != "::1" {
		t.Errorf("AAAA answers = %v", r.Answer)
	}
	if r = query(t, addr, "test", dns.TypeA); len(r.Answer) != 0 {
		t.Errorf("the bare TLD is not covered by address=/.test/: %v", r.Answer)
	}
}

func TestRefusesOtherNames(t *testing.T) {
	addr, _, stop := startTestServer(t, sampleConf)
	defer stop()
	if r := query(t, addr, "example.com", dns.TypeA); r.Rcode != dns.RcodeRefused {
		t.Errorf("rcode = %d, want REFUSED", r.Rcode)
	}
}

func TestNoDataForOtherTypes(t *testing.T) {
	addr, _, stop := startTestServer(t, sampleConf)
	defer stop()
	r := query(t, addr, "shop.test", dns.TypeMX)
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 0 {
		t.Errorf("MX: rcode=%d answers=%v, want empty NOERROR", r.Rcode, r.Answer)
	}
}

func TestPicksUpConfigChanges(t *testing.T) {
	addr, path, stop := startTestServer(t, sampleConf)
	defer stop()
	updated := "port=5300\naddress=/.test/192.168.1.20\n"
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future)

	r := query(t, addr, "shop.test", dns.TypeA)
	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "192.168.1.20" {
		t.Errorf("answers after rewrite = %v", r.Answer)
	}
}

func TestAnswersOverTCPToo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lerd.conf")
	if err := os.WriteFile(path, []byte(sampleConf), 0644); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(path)
	done := make(chan struct{})
	started := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.ServeTCP(ln, func() { close(started) })
	}()
	<-started
	defer func() { _ = srv.Shutdown(); <-done }()

	m := new(dns.Msg)
	m.SetQuestion("shop.test.", dns.TypeA)
	c := &dns.Client{Net: "tcp", Timeout: 2 * time.Second}
	r, _, err := c.Exchange(m, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("TCP answers = %v", r.Answer)
	}
}
