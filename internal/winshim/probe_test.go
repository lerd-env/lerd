package winshim

import (
	"net"
	"testing"
	"time"
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

func TestProbe_IgnoresAReplyToSomethingElse(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			buf[0]++ // a different query's id
			pc.WriteTo(buf[:n], addr)
		}
	}()
	if err := Probe(pc.LocalAddr().String(), "test", 100*time.Millisecond); err == nil {
		t.Fatal("Probe accepted a reply with the wrong id")
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
