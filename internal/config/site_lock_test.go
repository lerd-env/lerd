package config

import (
	"fmt"
	"os"
	"os/exec"
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
		_ = SetSiteIdleSuspendedWorkers("demo", []string{fmt.Sprint("queue-", i%2)})
	}
}

// The CLI pins a site while the watcher saves the same sites.yaml. Each loads
// the file, changes one field and writes it all back, so without a lock that
// spans processes the watcher's save put back the pin it had read a moment ago.
func TestSitePinSurvivesAWriterInAnotherProcess(t *testing.T) {
	real := os.Getenv("XDG_DATA_HOME")
	setDataDir(t)
	if err := AddSite(Site{Name: "demo", Domains: []string{"demo.test"}, Path: "/srv/demo"}); err != nil {
		t.Fatal(err)
	}
	writer := exec.Command(os.Args[0], "-test.run=^TestSitesWriterHelperProcess$")
	writer.Env = []string{"XDG_DATA_HOME=" + real, "GO_SITES_WRITER_DIR=" + os.Getenv("XDG_DATA_HOME")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "XDG_DATA_HOME=") {
			writer.Env = append(writer.Env, kv)
		}
	}
	if err := writer.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Process.Kill(); _ = writer.Wait() })
	time.Sleep(100 * time.Millisecond)

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
	if lost > 0 {
		t.Errorf("%d of 200 pin changes were overwritten by the other process", lost)
	}
}
