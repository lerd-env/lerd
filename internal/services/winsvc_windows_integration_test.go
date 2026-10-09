//go:build windows

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// TestRealContainerUnitMountsAWindowsPath drives the Windows service manager
// against the real Podman machine: a unit written the way lerd writes a site
// mount (the host path on both sides) has to start, see the host file at the
// mapped path, and accept an exec whose working directory is a Windows path.
// Opt-in, since it needs a running machine and pulls an image.
func TestRealContainerUnitMountsAWindowsPath(t *testing.T) {
	if os.Getenv("LERD_TEST_REAL_MACHINE") != "1" {
		t.Skip("set LERD_TEST_REAL_MACHINE=1 to run against the real Podman machine")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // lerd's own unit files; Podman keeps its state via its own dirs

	site := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(filepath.Join(site, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "public", "index.txt"), []byte("hello from windows"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &windowsServiceManager{}
	name := "lerd-mounttest"
	unit := "[Container]\nImage=docker.io/library/alpine:3\nContainerName=" + name + "\n" +
		"Volume=" + site + ":" + site + ":rw\nWorkingDir=" + site + "\nExec=sleep 300\n"
	if err := m.WriteContainerUnit(name, unit); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(name); _ = podman.Cmd("rm", "-f", name).Run() })
	if err := m.Start(name); err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < 20 && !podman.Cache.Running(name); i++ {
		time.Sleep(500 * time.Millisecond)
	}

	out, err := podman.Cmd("exec", "-w", filepath.Join(site, "public"), name, "cat", "index.txt").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "hello from windows" {
		t.Fatalf("exec with a Windows -w: err=%v out=%q", err, out)
	}
	if out, err := podman.Cmd("exec", name, "sh", "-c", "echo -n from-container > "+hostToVM(site)+"/public/back.txt").CombinedOutput(); err != nil {
		t.Fatalf("write through the mount: %v %s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(site, "public", "back.txt"))
	if err != nil || string(got) != "from-container" {
		t.Errorf("host did not see the container's write: %q %v", got, err)
	}
}

func hostToVM(p string) string {
	return "/mnt/c" + strings.ReplaceAll(p[2:], `\`, "/")
}
