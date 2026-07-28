package cli

import (
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func withTunnelRecordFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tunnels.json")
	prev := tunnelRecordPath
	tunnelRecordPath = func() string { return path }
	t.Cleanup(func() { tunnelRecordPath = prev })
	return path
}

func withProcessCommand(t *testing.T, fn func(int) string) {
	t.Helper()
	prev := processCommand
	processCommand = fn
	t.Cleanup(func() { processCommand = prev })
}

func TestRecordAndForgetTunnel(t *testing.T) {
	withTunnelRecordFile(t)

	recordTunnel(4242, []string{"cloudflared", "tunnel", "--url", "http://127.0.0.1:9000"})
	recordTunnel(4243, []string{"ngrok", "http", "80"})
	if got := readTunnelRecords(); len(got) != 2 {
		t.Fatalf("records = %v; want 2", got)
	}

	forgetTunnel(4242)
	got := readTunnelRecords()
	if len(got) != 1 || got[0].PID != 4243 {
		t.Fatalf("records after forget = %v; want only 4243", got)
	}

	forgetTunnel(4243)
	if got := readTunnelRecords(); len(got) != 0 {
		t.Fatalf("records = %v; want none", got)
	}
}

// The reap must not signal a pid that has been reused by something else.
func TestReapSkipsReusedPID(t *testing.T) {
	withTunnelRecordFile(t)
	recordTunnel(4242, []string{"cloudflared", "tunnel"})

	killed := false
	withProcessCommand(t, func(int) string { return "/usr/bin/some-other-program" })
	prev := tunnelKillFn
	tunnelKillFn = func(int, syscall.Signal) error { killed = true; return nil }
	t.Cleanup(func() { tunnelKillFn = prev })

	ReapOrphanTunnels()
	if killed {
		t.Error("signalled a pid whose process no longer matches the record")
	}
	if got := readTunnelRecords(); len(got) != 0 {
		t.Errorf("records = %v; want the file cleared", got)
	}
}

// A tunnel that outlived its lerd-ui is actually killed.
func TestReapKillsSurvivingTunnel(t *testing.T) {
	withTunnelRecordFile(t)

	cmd := exec.Command("/bin/sleep", "37")
	cmd.SysProcAttr = tunnelSysProcAttr()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }() //nolint:errcheck

	recordTunnel(cmd.Process.Pid, cmd.Args)
	ReapOrphanTunnels()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill() //nolint:errcheck
		t.Fatal("orphaned tunnel process was not killed by the reap")
	}
}
