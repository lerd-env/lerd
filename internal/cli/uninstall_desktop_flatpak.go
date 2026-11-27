package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/feedback"
)

// desktopFlatpakID is the Lerd desktop app install.sh offers, as a user flatpak.
const desktopFlatpakID = "sh.lerd.Desktop"

// desktopFlatpakInstalled reports the per-user install only: a system-wide one
// needs root to remove, and the installer never puts it there.
var desktopFlatpakInstalled = func() bool { return len(desktopFlatpakRefs()) > 0 }

// desktopFlatpakRefs lists every installed branch of the app. flatpak info and
// uninstall both refuse the bare app id once two branches are installed.
func desktopFlatpakRefs() []string {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return nil
	}
	out, err := exec.Command("flatpak", "list", "--user", "--app", "--columns=ref").Output()
	if err != nil {
		return nil
	}
	var refs []string
	for _, ref := range strings.Fields(string(out)) {
		if strings.HasPrefix(ref, desktopFlatpakID+"/") {
			refs = append(refs, ref)
		}
	}
	return refs
}

func confirmRemoveDesktopApp() bool {
	return feedback.Confirm("Remove the Lerd desktop app?", true)
}

// removeDesktopFlatpak closes the app first so it is not left running from a
// deleted install, and takes its sandbox data only when lerd's data goes too.
func removeDesktopFlatpak(deleteData bool) error {
	refs := desktopFlatpakRefs()
	if len(refs) == 0 {
		return fmt.Errorf("%s is not installed for this user", desktopFlatpakID)
	}
	_ = exec.Command("flatpak", "kill", desktopFlatpakID).Run()
	args := []string{"uninstall", "--user", "-y", "--noninteractive"}
	if deleteData {
		args = append(args, "--delete-data")
	}
	out, err := exec.Command("flatpak", append(args, refs...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
