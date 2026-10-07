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

// dnsmasq read anything in its conf dir; this server reads only what lerd
// writes, so a hand-added directive has to be named, not dropped in silence.
func TestParseConfCollectsUnsupportedLines(t *testing.T) {
	c, err := ParseConf([]byte("# comment\n\nport=5300\nno-resolv\nlog-queries\nserver=1.1.1.1\nserver=/corp/10.0.0.9\nserver=/odd/10.0.0.9@eth0\ncname=a.test,b.test\naddress=/.test/127.0.0.1\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"server=/odd/10.0.0.9@eth0", "cname=a.test,b.test"}
	if len(c.Unsupported) != len(want) || c.Unsupported[0] != want[0] || c.Unsupported[1] != want[1] {
		t.Errorf("unsupported = %q, want %q", c.Unsupported, want)
	}
}

func TestParseConfReadsPerDomainServers(t *testing.T) {
	c, err := ParseConf([]byte("server=/corp/10.0.0.9\nserver=/a.lan/b.lan/10.0.0.8#5353\nserver=/blocked/\nserver=/odd/10.0.0.9@eth0\n"))
	if err != nil {
		t.Fatal(err)
	}
	for domain, want := range map[string][]string{
		"corp":  {"10.0.0.9:53"},
		"a.lan": {"10.0.0.8:5353"},
		"b.lan": {"10.0.0.8:5353"},
	} {
		if got := c.Routes[domain]; len(got) != 1 || got[0] != want[0] {
			t.Errorf("route for %s = %v, want %v", domain, got, want)
		}
	}
	// A domain with no usable server must still be routed, to nowhere, so its
	// names are refused rather than leaked to the default upstream.
	for _, domain := range []string{"blocked", "odd"} {
		if got, ok := c.Routes[domain]; !ok || len(got) != 0 {
			t.Errorf("route for %s = %v (present %v), want an empty route", domain, got, ok)
		}
	}
	if len(c.Upstreams) != 0 {
		t.Errorf("per-domain servers must not become default upstreams: %v", c.Upstreams)
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
	srv := New(filepath.Dir(path))
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
	srv := New(filepath.Dir(path))
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
	return startUpstreamAnswering(t, "203.0.113.7")
}

func startUpstreamAnswering(t *testing.T, answer string) string {
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
			A:   net.ParseIP(answer).To4(),
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

// shortForwardTimeout keeps tests that wait on a silent upstream quick.
func shortForwardTimeout(t *testing.T) {
	t.Helper()
	prev := forwardTimeout
	forwardTimeout = 300 * time.Millisecond
	t.Cleanup(func() { forwardTimeout = prev })
}

// The dead upstreams below hold their port and never answer. A closed port
// would do in isolation, but under parallel packages another test's server
// can pick it up and answer.
func TestFallsThroughToTheNextUpstream(t *testing.T) {
	shortForwardTimeout(t)
	addr, _, stop := startTestServer(t, "port=5300\nserver="+hostPort(silentUpstream(t))+"\nserver="+hostPort(startUpstream(t))+"\n")
	defer stop()

	if r := query(t, addr, "example.com", dns.TypeA); r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 {
		t.Errorf("rcode=%d answers=%v, want the second upstream's answer", r.Rcode, r.Answer)
	}
}

func TestServfailWhenNoUpstreamAnswers(t *testing.T) {
	shortForwardTimeout(t)
	addr, _, stop := startTestServer(t, "port=5300\nserver="+hostPort(silentUpstream(t))+"\n")
	defer stop()

	if r := query(t, addr, "example.com", dns.TypeA); r.Rcode != dns.RcodeServerFailure {
		t.Errorf("rcode = %d, want SERVFAIL", r.Rcode)
	}
}

// silentUpstream accepts queries and never answers, the way an unreachable
// resolver behind a firewall looks.
func silentUpstream(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return pc.LocalAddr().String()
}

func TestAsksTheUpstreamThatLastAnsweredFirst(t *testing.T) {
	shortForwardTimeout(t)

	dh, dp, _ := net.SplitHostPort(silentUpstream(t))
	uh, up, _ := net.SplitHostPort(startUpstream(t))
	addr, _, stop := startTestServer(t, "port=5300\nserver="+dh+"#"+dp+"\nserver="+uh+"#"+up+"\n")
	defer stop()

	if r := query(t, addr, "example.com", dns.TypeA); len(r.Answer) != 1 {
		t.Fatalf("first query should fall through to the live upstream, got %v", r.Answer)
	}
	start := time.Now()
	if r := query(t, addr, "example.org", dns.TypeA); len(r.Answer) != 1 {
		t.Fatalf("second query: %v", r.Answer)
	}
	if took := time.Since(start); took >= forwardTimeout {
		t.Errorf("second query took %v, so it waited on the dead upstream again", took)
	}
}

func hostPort(addr string) string {
	h, p, _ := net.SplitHostPort(addr)
	return h + "#" + p
}

func TestRoutesADomainToItsOwnServer(t *testing.T) {
	public := startUpstreamAnswering(t, "203.0.113.7")
	corp := startUpstreamAnswering(t, "10.0.0.9")
	addr, _, stop := startTestServer(t, "port=5300\nserver="+hostPort(public)+"\nserver=/corp/"+hostPort(corp)+"\n")
	defer stop()

	if r := query(t, addr, "intranet.corp", dns.TypeA); len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "10.0.0.9" {
		t.Errorf("a corp name must go to the corp server, got %v", r.Answer)
	}
	if r := query(t, addr, "example.com", dns.TypeA); len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Errorf("other names keep the default upstream, got %v", r.Answer)
	}
}

// A private domain whose server lerd cannot use must not leak to the public
// default upstream.
func TestRefusesADomainRoutedNowhere(t *testing.T) {
	public := startUpstreamAnswering(t, "203.0.113.7")
	addr, _, stop := startTestServer(t, "port=5300\nserver="+hostPort(public)+"\nserver=/corp/10.0.0.9@eth0\n")
	defer stop()

	if r := query(t, addr, "intranet.corp", dns.TypeA); r.Rcode != dns.RcodeRefused {
		t.Errorf("rcode = %d, want REFUSED", r.Rcode)
	}
}

// dnsmasq read every file in its conf dir, so a rule someone dropped next to
// lerd.conf has to keep working, while editor backups stay ignored.
func TestReadsEveryFileInTheDirectory(t *testing.T) {
	public := startUpstreamAnswering(t, "203.0.113.7")
	corp := startUpstreamAnswering(t, "10.0.0.9")
	addr, path, stop := startTestServer(t, "port=5300\nserver="+hostPort(public)+"\n")
	defer stop()
	dir := filepath.Dir(path)
	if err := os.WriteFile(filepath.Join(dir, "corp.conf"), []byte("server=/corp/"+hostPort(corp)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corp.conf~"), []byte("server=/corp/#\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if r := query(t, addr, "intranet.corp", dns.TypeA); len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "10.0.0.9" {
		t.Errorf("a rule in another file must apply, got rcode=%d %v", r.Rcode, r.Answer)
	}
}

// dnsmasq reads server=/#/ip as a default upstream for every domain, so it
// must not end up as a route for a domain literally called "#".
func TestHashDomainIsADefaultUpstream(t *testing.T) {
	c, err := ParseConf([]byte("server=/#/10.0.0.9\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Upstreams) != 1 || c.Upstreams[0] != "10.0.0.9:53" {
		t.Errorf("upstreams = %v, want 10.0.0.9:53", c.Upstreams)
	}
	if _, ok := c.Routes["#"]; ok {
		t.Error("# must not become a route")
	}
}

// server=/d/# hands d back to the default upstreams, which is how dnsmasq
// exempts a subdomain from a broader per-domain rule.
func TestHashTargetUsesTheDefaultUpstreams(t *testing.T) {
	public := startUpstreamAnswering(t, "203.0.113.7")
	corp := startUpstreamAnswering(t, "10.0.0.9")
	addr, _, stop := startTestServer(t, "port=5300\nserver="+hostPort(public)+"\nserver=/corp/"+hostPort(corp)+"\nserver=/public.corp/#\n")
	defer stop()

	if r := query(t, addr, "www.public.corp", dns.TypeA); len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Errorf("public.corp must use the default upstream, got rcode=%d %v", r.Rcode, r.Answer)
	}
	if r := query(t, addr, "intranet.corp", dns.TypeA); len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "10.0.0.9" {
		t.Errorf("the rest of corp keeps its own server, got %v", r.Answer)
	}
}
