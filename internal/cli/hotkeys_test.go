package cli

import (
	"os"
	"testing"
	"time"
)

// TestHotkeysReportKeys checks the reader forwards what is typed while it runs.
func TestHotkeysReportKeys(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close() //nolint:errcheck
	defer w.Close() //nolint:errcheck

	got := make(chan byte, 4)
	k := startHotkeys(int(r.Fd()), func(b byte) { got <- b })
	if k == nil {
		t.Fatal("startHotkeys returned nil")
	}
	defer k.stop()

	if _, err := w.Write([]byte{0x0F}); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case b := <-got:
		if b != 0x0F {
			t.Fatalf("got byte %#x, want 0x0F", b)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("keypress never reported")
	}
}

// TestHotkeysLeaveInputAfterStop is the regression guard for the install
// freeze: a stopped reader must not still be queued on the terminal, or the
// shim prompt that follows loses the answer typed at it.
func TestHotkeysLeaveInputAfterStop(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close() //nolint:errcheck
	defer w.Close() //nolint:errcheck

	k := startHotkeys(int(r.Fd()), func(byte) {})
	if k == nil {
		t.Fatal("startHotkeys returned nil")
	}
	k.stop()

	if _, err := w.Write([]byte("y\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 2)
	if err := r.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read after stop: %v", err)
	}
	if string(buf[:n]) != "y\n" {
		t.Fatalf("read %q after stop, want %q", string(buf[:n]), "y\n")
	}
}
