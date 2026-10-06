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

func TestParseConfReadsUpstreams(t *testing.T) {
	c, err := ParseConf([]byte("server=1.1.1.1\nserver=10.0.0.2#5353\nserver=2606:4700::1111\nserver=/corp/10.0.0.9\nserver=nope\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.1.1.1:53", "10.0.0.2:5353", "[2606:4700::1111]:53"}
	if len(c.Upstreams) != len(want) {
		t.Fatalf("upstreams = %v, want %v", c.Upstreams, want)
	}
	for i := range want {
		if c.Upstreams[i] != want[i] {
			t.Errorf("upstreams[%d] = %s, want %s", i, c.Upstreams[i], want[i])
		}
	}
}

func TestParseConfReadsLogQueries(t *testing.T) {
	c, err := ParseConf([]byte("port=5300\nlog-queries\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.LogQueries {
		t.Error("log-queries must switch query logging on, the dashboard tells users to add it")
	}
	if c, _ = ParseConf([]byte(sampleConf)); c.LogQueries {
		t.Error("query logging must stay off by default")
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

func TestRefusesOtherNamesWithoutUpstreams(t *testing.T) {
	addr, _, stop := startTestServer(t, "port=5300\naddress=/.test/127.0.0.1\n")
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

// startUpstream runs a stand-in for the user's real resolver that answers
// every A query with 203.0.113.7.
func startUpstream(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	srv := &dns.Server{PacketConn: pc, NotifyStartedFunc: func() { close(started) }, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.ParseIP("203.0.113.7").To4(),
		})
		_ = w.WriteMsg(resp)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

func TestForwardsOtherNamesToUpstream(t *testing.T) {
	up := startUpstream(t)
	host, port, _ := net.SplitHostPort(up)
	addr, _, stop := startTestServer(t, "port=5300\nno-resolv\nserver="+host+"#"+port+"\naddress=/.test/127.0.0.1\n")
	defer stop()

	r := query(t, addr, "example.com", dns.TypeA)
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatalf("forwarded answer: rcode=%d answers=%v", r.Rcode, r.Answer)
	}
	if r = query(t, addr, "shop.test", dns.TypeA); len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("the lerd domain must still be answered locally, got %v", r.Answer)
	}
}

func TestFallsThroughToTheNextUpstream(t *testing.T) {
	dead, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := dead.LocalAddr().String()
	_ = dead.Close()
	dh, dp, _ := net.SplitHostPort(deadAddr)
	uh, upPort, _ := net.SplitHostPort(startUpstream(t))
	addr, _, stop := startTestServer(t, "port=5300\nserver="+dh+"#"+dp+"\nserver="+uh+"#"+upPort+"\n")
	defer stop()

	if r := query(t, addr, "example.com", dns.TypeA); r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 {
		t.Errorf("rcode=%d answers=%v, want the second upstream's answer", r.Rcode, r.Answer)
	}
}

func TestServfailWhenNoUpstreamAnswers(t *testing.T) {
	dead, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h, p, _ := net.SplitHostPort(dead.LocalAddr().String())
	_ = dead.Close()
	addr, _, stop := startTestServer(t, "port=5300\nserver="+h+"#"+p+"\n")
	defer stop()

	if r := query(t, addr, "example.com", dns.TypeA); r.Rcode != dns.RcodeServerFailure {
		t.Errorf("rcode = %d, want SERVFAIL", r.Rcode)
	}
}
