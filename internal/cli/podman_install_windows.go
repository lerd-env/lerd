//go:build windows

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// podmanInstallDirs are where Podman's installers put podman.exe: the MSI per
// user and for all users, and the older setup.exe bundle.
func podmanInstallDirs() []string {
	return []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Podman"),
		filepath.Join(os.Getenv("ProgramFiles"), "Podman"),
		filepath.Join(os.Getenv("ProgramFiles"), "RedHat", "Podman"),
	}
}

// findPodman reports whether podman can be run, putting an installed one on
// this process's PATH. A terminal opened before the install still has the old
// PATH, so podman missing from it does not mean Podman is missing.
func findPodman(dirs []string) bool {
	if _, err := exec.LookPath("podman"); err == nil {
		return true
	}
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, "podman.exe")); err == nil {
			os.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH")) //nolint:errcheck
			return true
		}
	}
	return false
}

// ensurePodmanCLI installs the pinned Podman MSI when the host has no Podman.
// The MSI installs for the current user, so no elevation is needed, and it adds
// Podman to the user PATH for terminals opened afterwards.
func ensurePodmanCLI(w io.Writer) error {
	if findPodman(podmanInstallDirs()) {
		return nil
	}
	feedback.Line("Podman is not installed, installing it…")
	msi := filepath.Join(os.TempDir(), "lerd-podman-installer.msi")
	defer os.Remove(msi)
	var pins pinnedTools
	if _, err := pins.download("podman", msi, 0o644, w); err != nil {
		return fmt.Errorf("podman download: %w", err)
	}
	logDir := filepath.Join(config.DataDir(), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(logDir, "podman-install.log")
	err := exec.Command("msiexec.exe", "/i", msi, "/qn", "/norestart", "/l*v", logPath).Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		return fmt.Errorf("running the Podman installer: %w", err)
	}
	if err := msiResult(code, logPath); err != nil {
		return err
	}
	if !findPodman(podmanInstallDirs()) {
		return fmt.Errorf("the Podman installer finished but podman.exe is not where it installs to; see %s", logPath)
	}
	return nil
}

// msiResult turns a msiexec exit code into an error. 3010 is success with a
// reboot pending, which Podman's CLI does not need.
func msiResult(code int, logPath string) error {
	switch code {
	case 0, 3010:
		return nil
	case 1602:
		return fmt.Errorf("the Podman install was cancelled; see %s", logPath)
	case 1618:
		return fmt.Errorf("another installation is in progress, wait for it to finish and run lerd install again; see %s", logPath)
	}
	return fmt.Errorf("the Podman installer failed with exit code %d; see %s", code, logPath)
}
