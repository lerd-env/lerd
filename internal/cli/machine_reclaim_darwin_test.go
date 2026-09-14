//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReclaimScriptVacuumsJournalAndTrims(t *testing.T) {
	script := reclaimScript()
	for _, want := range []string{
		journalCapDropIn,
		"SystemMaxUse=" + strconv.Itoa(journalCapMiB) + "M",
		"--vacuum-size=" + strconv.Itoa(journalCapMiB) + "M",
		"fstrim",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("reclaim script missing %q:\n%s", want, script)
		}
	}
}

func TestJournalCapScriptIsIdempotent(t *testing.T) {
	script := journalCapScript()
	if !strings.Contains(script, "test -f "+journalCapDropIn) {
		t.Errorf("journal cap script must skip an already-capped VM:\n%s", script)
	}
	if strings.Contains(script, "fstrim") {
		t.Error("the start-path cap must not trim; that is what lerd machine reclaim is for")
	}
}

// The VM image is a sparse 100 GiB file, so only the blocks it actually
// allocates say how much host disk it holds.
func TestMachineImageBytesReportsAllocatedNotApparent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".local", "share", "containers", "podman", "machine", "applehv")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "podman-machine-default-arm64.raw")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	const apparent = 64 << 20
	if err := f.Truncate(apparent); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := machineImageBytes("podman-machine-default")
	if err != nil {
		t.Fatalf("machineImageBytes: %v", err)
	}
	if got >= apparent {
		t.Errorf("sparse image reported as %d bytes, want well under the apparent %d", got, apparent)
	}
}

func TestMachineImageBytesMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := machineImageBytes("podman-machine-default"); err == nil {
		t.Error("a machine with no disk image must report an error, not a zero size")
	}
}

func TestReclaimedReport(t *testing.T) {
	cases := []struct {
		name          string
		before, after int64
		want          string
	}{
		{"freed", 22 << 30, 12 << 30, "Freed 10.0 GiB"},
		{"nothing", 12 << 30, 12 << 30, "Nothing to reclaim"},
		{"grew", 12 << 30, 13 << 30, "Nothing to reclaim"},
		{"unknown", 0, 0, "Nothing to reclaim"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reclaimedReport(c.before, c.after); !strings.Contains(got, c.want) {
				t.Errorf("reclaimedReport(%d, %d) = %q, want it to contain %q", c.before, c.after, got, c.want)
			}
		})
	}
}
