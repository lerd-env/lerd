package winshim

import (
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// answering stands in for the agent: a real socket running the real Answer.
func answering(t *testing.T, tld string) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if resp, err := Answer(buf[:n], tld); err == nil {
				pc.WriteTo(resp, addr)
			}
		}
	}()
	return pc.LocalAddr().String()
}

func TestProbe_PassesWhenTheAgentAnswers(t *testing.T) {
	if err := Probe(answering(t, "test"), "test", time.Second); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

// A bound socket that receives nothing is what a DNS filter on the host leaves.
func TestProbe_FailsWhenTheQueryNeverArrives(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	if err := Probe(pc.LocalAddr().String(), "test", 100*time.Millisecond); err == nil {
		t.Fatal("Probe passed with nothing answering")
	}
}

// replying stands in for something answering in the agent's place: reply gets
// the real query and returns the bytes to send back.
func replying(t *testing.T, reply func(query []byte) []byte) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(reply(buf[:n]), addr)
		}
	}()
	return pc.LocalAddr().String()
}

func answerFor(t *testing.T, query []byte, tld string, edit func(*dnsmessage.Message)) []byte {
	t.Helper()
	resp, err := Answer(query, tld)
	if err != nil {
		t.Fatal(err)
	}
	var m dnsmessage.Message
	if err := m.Unpack(resp); err != nil {
		t.Fatal(err)
	}
	edit(&m)
	out, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProbe_RejectsAReplyThatIsNotTheAgents(t *testing.T) {
	for name, edit := range map[string]func(*dnsmessage.Message){
		"a different query's id": func(m *dnsmessage.Message) { m.ID++ },
		"name error": func(m *dnsmessage.Message) {
			m.RCode = dnsmessage.RCodeNameError
			m.Answers = nil
		},
		"refused": func(m *dnsmessage.Message) {
			m.RCode = dnsmessage.RCodeRefused
			m.Answers = nil
		},
		"no answer": func(m *dnsmessage.Message) { m.Answers = nil },
		"somewhere else": func(m *dnsmessage.Message) {
			m.Answers[0].Body = &dnsmessage.AResource{A: [4]byte{203, 0, 113, 7}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			addr := replying(t, func(q []byte) []byte { return answerFor(t, q, "test", edit) })
			if err := Probe(addr, "test", 100*time.Millisecond); err == nil {
				t.Fatal("Probe accepted it")
			}
		})
	}
}

// The DNS name limit leaves a 245 character suffix room for a short label only.
func TestProbe_FitsALongTLD(t *testing.T) {
	label := strings.Repeat("a", 60)
	tld := strings.Join([]string{label, label, label, label[:60]}, ".")
	if err := Probe(answering(t, tld), tld, time.Second); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestProbeFailedIn(t *testing.T) {
	for _, c := range []struct {
		name, log string
		want      bool
	}{
		{"no log yet", "", false},
		{"agent older than the self-test", "2026/11/06 answering *.test on 127.0.0.1:53\n", false},
		{"passed", "x answering\nx " + ProbePassed + "\n", false},
		{"failed", "x answering\nx " + ProbeFailed + ": no answer\n", true},
		{"failed, then passed after a retry", "x " + ProbeFailed + ": no answer\nx " + ProbePassed + "\n", false},
		{"passed, then failed", "x " + ProbePassed + "\nx " + ProbeFailed + ": no answer\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ProbeFailedIn(c.log); got != c.want {
				t.Errorf("ProbeFailedIn(%q) = %v, want %v", c.log, got, c.want)
			}
		})
	}
}
