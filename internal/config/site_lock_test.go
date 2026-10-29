package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSitesWriterHelperProcess is the second process in the test below, the
// watcher's idle engine saving a site's suspended workers over and over.
func TestSitesWriterHelperProcess(t *testing.T) {
	dir := os.Getenv("GO_SITES_WRITER_DIR")
	if dir == "" {
		return
	}
	// Set here rather than inherited: the guard takes the data dir a test process
	// starts with as the developer's real one and refuses to write to it.
	t.Setenv("XDG_DATA_HOME", dir)
	for i := 0; ; i++ {
		if err := SetSiteIdleSuspendedWorkers("demo", []string{fmt.Sprint("queue-", i%2)}); err != nil {
			fmt.Fprintln(os.Stderr, "writer:", err)
			os.Exit(1)
		}
		if i == 0 {
			_ = os.WriteFile(filepath.Join(dir, "writer-ready"), nil, 0644)
		}
	}
}

// The CLI pins a site while the watcher saves the same sites.yaml. Each loads
// the file, changes one field and writes it all back, so without a lock that
// spans processes the watcher's save put back the pin it had read a moment ago.
func TestSitePinSurvivesAWriterInAnotherProcess(t *testing.T) {
	real := os.Getenv("XDG_DATA_HOME")
	setDataDir(t)
	dir := os.Getenv("XDG_DATA_HOME")
	if err := AddSite(Site{Name: "demo", Domains: []string{"demo.test"}, Path: "/srv/demo"}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	writer := exec.Command(os.Args[0], "-test.run=^TestSitesWriterHelperProcess$")
	writer.Stdout, writer.Stderr = &out, &out
	writer.Env = []string{"XDG_DATA_HOME=" + real, "GO_SITES_WRITER_DIR=" + dir}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "XDG_DATA_HOME=") {
			writer.Env = append(writer.Env, kv)
		}
	}
	if err := writer.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- writer.Wait() }()
	t.Cleanup(func() { _ = writer.Process.Kill(); <-exited })

	for deadline := time.Now().Add(10 * time.Second); ; {
		if _, err := os.Stat(filepath.Join(dir, "writer-ready")); err == nil {
			break
		}
		select {
		case err := <-exited:
			t.Fatalf("writer exited before its first save (%v): %s", err, out.String())
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("writer never saved: %s", out.String())
		}
	}

	lost := 0
	for i := 0; i < 200; i++ {
		want := i%2 == 0
		if err := SetSitePinned("demo", want); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
		if s, err := FindSite("demo"); err != nil || s.Pinned != want {
			lost++
		}
	}
	select {
	case err := <-exited:
		t.Fatalf("writer stopped mid-test (%v): %s", err, out.String())
	default:
	}
	if lost > 0 {
		t.Errorf("%d of 200 pin changes were overwritten by the other process", lost)
	}
}
