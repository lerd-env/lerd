//go:build windows

package p9share

import (
	"os/exec"
	"testing"
	"time"
)

func TestProcessExitFiresWhenThePIDEnds(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited, err := processExit(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("processExit never fired")
	}
}

func TestServeRefusesAPIDThatIsGone(t *testing.T) {
	if err := Serve([]Share{{Dir: t.TempDir(), Service: "0000e36a-facb-11e6-bd58-64006a7986d3"}}, 0x7ffffff0); err == nil {
		t.Error("Serve should refuse a PID it cannot watch")
	}
}
