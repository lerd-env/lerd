//go:build darwin

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// journalCapMiB bounds the VM's systemd journal. Nothing inside the machine is
// worth keeping weeks of: it logs container and machine noise only, and an
// uncapped journal reached 4 GB on a five-day-old VM.
const journalCapMiB = 200

// journalCapDropIn is where the cap lives inside the VM. Its presence is also
// the "already capped" marker, so the start path costs one `test -f`.
const journalCapDropIn = "/etc/systemd/journald.conf.d/lerd-journal-size.conf"

// journalCapScript caps the VM journal and vacuums whatever it had already
// grown to. Runs on every start, so everything sits behind the drop-in check.
func journalCapScript() string {
	size := strconv.Itoa(journalCapMiB) + "M"
	return fmt.Sprintf(`if ! test -f %s; then
sudo mkdir -p %s
printf '[Journal]\nSystemMaxUse=%s\n' | sudo tee %s >/dev/null
sudo systemctl restart systemd-journald
sudo journalctl --vacuum-size=%s >/dev/null 2>&1
fi`, journalCapDropIn, filepath.Dir(journalCapDropIn), size, journalCapDropIn, size)
}

// reclaimScript caps the journal, then trims the guest filesystem. Without the
// trim, blocks the VM frees are never returned to the host: the machine's disk
// image is a sparse raw file that only ever grows, so a VM using 14 GB can hold
// 22 GB of the host disk.
func reclaimScript() string {
	return journalCapScript() + "\nsudo fstrim /"
}

// applyJournalCap installs the journal cap on an already-running machine,
// best-effort: a VM that logs too much is not a reason to fail a start.
func applyJournalCap(name string) {
	if name == "" {
		return
	}
	_ = podman.Cmd("machine", "ssh", name, journalCapScript()).Run()
}

// machineImageBytes returns the host disk the named machine's image actually
// occupies. Stat's apparent size is the VM's provisioned 100 GiB, so the
// allocated blocks are the only number that answers "how much disk is this".
func machineImageBytes(name string) (int64, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	pattern := filepath.Join(home, ".local", "share", "containers", "podman", "machine", "*", name+"*.raw")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return 0, fmt.Errorf("no disk image found for podman machine %q", name)
	}
	info, err := os.Stat(matches[0])
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("cannot read allocated blocks of %s", matches[0])
	}
	return int64(st.Blocks) * 512, nil
}

// reclaimedReport renders the before/after image sizes as one line. A size we
// could not read is reported as nothing reclaimed rather than as a bogus total.
func reclaimedReport(before, after int64) string {
	if before <= 0 || after <= 0 {
		return "Nothing to reclaim."
	}
	if after >= before {
		return fmt.Sprintf("Nothing to reclaim; the VM disk image still holds %s.", humanSize(after))
	}
	return fmt.Sprintf("Freed %s; the VM disk image now holds %s.", humanSize(before-after), humanSize(after))
}

// runMachineReclaim returns disk the VM is holding but no longer using.
func runMachineReclaim() error {
	name := selectedMachineName()
	if name == "" {
		fmt.Println("No Podman Machine found; nothing to reclaim. Run `lerd start` to create one.")
		return nil
	}

	before, _ := machineImageBytes(name)
	feedback.Line("Vacuuming the VM journal and trimming its filesystem…")
	cmd := podman.Cmd("machine", "ssh", name, reclaimScript())
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reclaim disk in podman machine %s: %w", name, err)
	}

	after, _ := machineImageBytes(name)
	feedback.Line(reclaimedReport(before, after))
	return nil
}
