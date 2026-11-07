package winshim

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// The agent logs one of these after its self-test, and lerd doctor reads the
// log from inside the distro, which has no other way to see the Windows side.
const (
	ProbePassed = "DNS self-test passed"
	ProbeFailed = "DNS self-test failed"
)

// probeID marks the self-test's query, so its answer is told from anyone else's.
const probeID = 0x1e4d

// Probe sends one real query to addr and waits for its answer. A bind proves
// nothing on its own: a DNS filter on the host can swallow every well-formed
// query on port 53 while the socket looks healthy.
func Probe(addr, tld string, timeout time.Duration) error {
	name, err := dnsmessage.NewName("lerd-self-test." + tld + ".")
	if err != nil {
		return err
	}
	query, err := (&dnsmessage.Message{
		Header:    dnsmessage.Header{ID: probeID, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}).Pack()
	if err != nil {
		return err
	}
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	if _, err := conn.Write(query); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("no answer from %s: %w", addr, err)
		}
		var p dnsmessage.Parser
		if h, err := p.Start(buf[:n]); err == nil && h.Response && h.ID == probeID {
			return nil
		}
	}
}

// ProbeFailedIn reports whether the agent's latest self-test in its log failed.
// A log with no verdict, from an older lerd.exe, is not a failure.
func ProbeFailedIn(agentLog string) bool {
	return strings.LastIndex(agentLog, ProbeFailed) > strings.LastIndex(agentLog, ProbePassed)
}

// Answer replies to one DNS query: every name under tld is loopback, where nginx
// listens through mirrored networking, and anything else is refused. Answering
// DNS rather than writing hosts lines gives wildcards for free.
func Answer(query []byte, tld string) ([]byte, error) {
	var p dnsmessage.Parser
	h, err := p.Start(query)
	if err != nil {
		return nil, err
	}
	q, err := p.Question()
	if err != nil {
		return nil, err
	}
	resp := dnsmessage.Header{ID: h.ID, Response: true, Authoritative: true, RecursionDesired: h.RecursionDesired}
	owned := underTLD(q.Name.String(), tld)
	if !owned {
		resp.RCode = dnsmessage.RCodeRefused
	}
	b := dnsmessage.NewBuilder(nil, resp)
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(q); err != nil {
		return nil, err
	}
	if err := b.StartAnswers(); err != nil {
		return nil, err
	}
	rh := dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60}
	switch {
	case owned && q.Type == dnsmessage.TypeA:
		err = b.AResource(rh, dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}})
	case owned && q.Type == dnsmessage.TypeAAAA:
		err = b.AAAAResource(rh, dnsmessage.AAAAResource{AAAA: [16]byte{15: 1}})
	}
	if err != nil {
		return nil, err
	}
	return b.Finish()
}

func underTLD(name, tld string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	tld = strings.ToLower(tld)
	return tld != "" && (name == tld || strings.HasSuffix(name, "."+tld))
}
