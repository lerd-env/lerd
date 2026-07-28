package cli

import (
	"os"
	"testing"
)

// Every test here writes under the run dir, so point the data dir at a temp one.
func tunnelStateHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func TestTunnelState_roundTrip(t *testing.T) {
	tunnelStateHome(t)

	st := tunnelState{Site: "acme", Tool: "cloudflare", URL: "https://x.trycloudflare.com", PID: os.Getpid()}
	if err := writeTunnelState("acme", st); err != nil {
		t.Fatalf("writing: %v", err)
	}

	info, ok := cliTunnelStatus("acme")
	if !ok {
		t.Fatal("a live CLI tunnel was not reported")
	}
	if info.URL != st.URL || info.Tool != st.Tool {
		t.Errorf("info = %+v, want the recorded tool and URL", info)
	}
	if !info.External {
		t.Error("a CLI tunnel must be marked external so the UI can say where it came from")
	}

	removeTunnelState("acme")
	if _, ok := cliTunnelStatus("acme"); ok {
		t.Error("the entry is still reported after being removed")
	}
}

// A share killed outright never gets to clean up, so the reader is what clears
// the entry rather than leaving a dead tunnel on the dashboard forever.
func TestReadTunnelStates_dropsEntriesWhoseProcessIsGone(t *testing.T) {
	tunnelStateHome(t)

	dead := tunnelState{Site: "acme", Tool: "serveo", URL: "https://x.serveo.net", PID: 2 << 30}
	if err := writeTunnelState("acme", dead); err != nil {
		t.Fatalf("writing: %v", err)
	}

	if _, ok := cliTunnelStatus("acme"); ok {
		t.Error("an entry whose process is gone must not be reported")
	}
	if _, err := os.Stat(tunnelStateFile("acme")); !os.IsNotExist(err) {
		t.Error("the stale file should have been removed")
	}
}

func TestReadTunnelStates_keysAWorktreeSeparately(t *testing.T) {
	tunnelStateHome(t)

	site := tunnelState{Site: "acme", Tool: "serveo", URL: "https://site.serveo.net", PID: os.Getpid()}
	branch := tunnelState{Site: "acme", Branch: "feat", Tool: "serveo", URL: "https://branch.serveo.net", PID: os.Getpid()}
	if err := writeTunnelState(tunnelKey("acme", ""), site); err != nil {
		t.Fatalf("writing site: %v", err)
	}
	if err := writeTunnelState(tunnelKey("acme", "feat"), branch); err != nil {
		t.Fatalf("writing branch: %v", err)
	}

	got, ok := cliTunnelStatus(tunnelKey("acme", "feat"))
	if !ok || got.URL != branch.URL {
		t.Errorf("branch tunnel = %+v, want %q", got, branch.URL)
	}
	got, ok = cliTunnelStatus(tunnelKey("acme", ""))
	if !ok || got.URL != site.URL {
		t.Errorf("site tunnel = %+v, want %q", got, site.URL)
	}
}

// A branch name a path cannot carry still has to land in a file of its own.
func TestTunnelStateFile_survivesASlashInTheBranch(t *testing.T) {
	tunnelStateHome(t)

	st := tunnelState{Site: "acme", Branch: "feat/login", Tool: "serveo", URL: "https://x.serveo.net", PID: os.Getpid()}
	if err := writeTunnelState(tunnelKey("acme", "feat/login"), st); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, ok := cliTunnelStatus(tunnelKey("acme", "feat/login")); !ok {
		t.Error("a branch with a slash in it was not readable back")
	}
}

func TestTunnelStateWriter_publishesTheURLItSeesInTheOutput(t *testing.T) {
	tunnelStateHome(t)

	w := newTunnelStateWriter("acme", tunnelState{Site: "acme", Tool: "serveo", PID: os.Getpid()})
	if _, ok := cliTunnelStatus("acme"); ok {
		t.Fatal("nothing should be published before a URL is known")
	}

	if _, err := w.Write([]byte("Forwarding HTTP traffic from https://abc.serveo.net\n")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	info, ok := cliTunnelStatus("acme")
	if !ok || info.URL != "https://abc.serveo.net" {
		t.Errorf("published %+v, want the URL from the output", info)
	}

	w.clear()
	if _, ok := cliTunnelStatus("acme"); ok {
		t.Error("the entry outlived the share")
	}
}

// The URL rarely arrives in one write, so the scan has to work across chunks.
func TestTunnelStateWriter_joinsSplitWrites(t *testing.T) {
	tunnelStateHome(t)

	w := newTunnelStateWriter("acme", tunnelState{Site: "acme", Tool: "serveo", PID: os.Getpid()})
	for _, chunk := range []string{"Forwarding from ", "https://abc.serveo", ".net\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatalf("writing %q: %v", chunk, err)
		}
	}
	info, ok := cliTunnelStatus("acme")
	if !ok || info.URL != "https://abc.serveo.net" {
		t.Errorf("published %+v, want the URL assembled from both writes", info)
	}
}

// A named Cloudflare tunnel knows its hostname up front, so the dashboard sees
// it without anything having to be parsed out of cloudflared's logs.
func TestTunnelStateWriter_publishesAKnownURLImmediately(t *testing.T) {
	tunnelStateHome(t)

	newTunnelStateWriter("acme", tunnelState{
		Site: "acme", Tool: "cloudflare", URL: "https://acme.example.com", PID: os.Getpid(),
	})
	info, ok := cliTunnelStatus("acme")
	if !ok || info.URL != "https://acme.example.com" {
		t.Errorf("published %+v, want the hostname it already knew", info)
	}
}

func TestStopCLITunnel_reportsWhetherThereWasOne(t *testing.T) {
	tunnelStateHome(t)

	if stopCLITunnel("acme") {
		t.Error("stopping a tunnel that is not running should report nothing was there")
	}
}

// A branch share must not be reported against the parent site, or the
// dashboard would show the site itself as shared.
func TestCliTunnelStatus_branchShareIsNotTheSiteShare(t *testing.T) {
	tunnelStateHome(t)

	st := tunnelState{
		Site: "acme", Branch: "feat", Tool: "cloudflare",
		URL: "https://feat-acme.example.com", PID: os.Getpid(),
	}
	if err := writeTunnelState(tunnelKey("acme", "feat"), st); err != nil {
		t.Fatalf("writing: %v", err)
	}

	if _, ok := cliTunnelStatus(tunnelKey("acme", "")); ok {
		t.Error("a branch share was reported against the site itself")
	}
	if got, ok := cliTunnelStatus(tunnelKey("acme", "feat")); !ok || got.URL != st.URL {
		t.Errorf("branch share = %+v, %v; want %q", got, ok, st.URL)
	}
}
