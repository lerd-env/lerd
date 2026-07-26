package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePortDropIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "99-lerd-ports.conf")

	var gotName string
	var gotArgs []string
	run := func(name string, args ...string) error {
		gotName = name
		gotArgs = args
		return nil
	}

	if err := writePortDropIn(path, run); err != nil {
		t.Fatalf("writePortDropIn: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("drop-in not written: %v", err)
	}
	if string(data) != unprivPortSetting+"\n" {
		t.Errorf("drop-in content = %q, want %q", string(data), unprivPortSetting+"\n")
	}
	if gotName != "sysctl" || len(gotArgs) != 2 || gotArgs[0] != "-w" || gotArgs[1] != unprivPortSetting {
		t.Errorf("runner called with %q %v, want sysctl -w %s", gotName, gotArgs, unprivPortSetting)
	}
}

func TestEnableLinger(t *testing.T) {
	var gotArgs []string
	run := func(_ string, args ...string) error {
		gotArgs = args
		return nil
	}

	// A blank user is a no-op: nothing to enable, no runner call.
	if err := enableLinger("", run); err != nil {
		t.Fatalf("enableLinger blank: %v", err)
	}
	if gotArgs != nil {
		t.Errorf("blank user still called runner: %v", gotArgs)
	}

	if err := enableLinger("george", run); err != nil {
		t.Fatalf("enableLinger: %v", err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "enable-linger" || gotArgs[1] != "george" {
		t.Errorf("runner args = %v, want enable-linger george", gotArgs)
	}
}

func TestBootstrapTargetUser(t *testing.T) {
	t.Setenv("SUDO_USER", "sudoer")
	t.Setenv("USER", "current")

	if got := bootstrapTargetUser("explicit"); got != "explicit" {
		t.Errorf("flag user ignored: got %q", got)
	}
	if got := bootstrapTargetUser(""); got != "sudoer" {
		t.Errorf("SUDO_USER not preferred: got %q", got)
	}

	// root as SUDO_USER is meaningless (it means apt was run by root directly),
	// so fall back to USER.
	t.Setenv("SUDO_USER", "root")
	if got := bootstrapTargetUser(""); got != "current" {
		t.Errorf("root SUDO_USER not skipped: got %q", got)
	}
}
