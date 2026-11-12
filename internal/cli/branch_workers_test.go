package cli

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// statefulLifecycle tracks which units run, and logs each call in order.
type statefulLifecycle struct {
	active map[string]bool
	calls  []string
}

func (f *statefulLifecycle) Start(name string) error {
	f.active[name] = true
	f.calls = append(f.calls, "start "+name)
	return nil
}
func (f *statefulLifecycle) Stop(name string) error {
	f.active[name] = false
	f.calls = append(f.calls, "stop "+name)
	return nil
}
func (f *statefulLifecycle) Restart(name string) error {
	f.calls = append(f.calls, "restart "+name)
	return nil
}
func (f *statefulLifecycle) UnitStatus(name string) (string, error) {
	if f.active[name] {
		return "active", nil
	}
	return "inactive", nil
}
func (f *statefulLifecycle) AllUnitStates() map[string]string { return nil }

// npm ci replaces node_modules: a vite left running serves stale bundles and
// loses the config its unit starts it with, so its next restart fails. It has
// to be stopped around the install and started again, writing that config back.
func TestInstallJSRestartsRunningDevServerWithItsConfig(t *testing.T) {
	dir := devServerSite(t, false)
	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	tool := config.DevServerToolInstalled(dir)
	wrapper := filepath.Join(dir, tool.WrapperPath)

	fake := &statefulLifecycle{active: map[string]bool{"lerd-vite-myapp": true, "lerd-queue-myapp": true}}
	podman.UnitLifecycle = fake
	t.Cleanup(func() { podman.UnitLifecycle = nil })

	prev := installJSDeps
	installJSDeps = func(path string, _ io.Writer) error {
		fake.calls = append(fake.calls, "install")
		return os.RemoveAll(filepath.Join(path, "node_modules", ".lerd"))
	}
	t.Cleanup(func() { installJSDeps = prev })

	fw := func() *config.Framework {
		f, _ := config.GetFrameworkForDir(site.Framework, dir)
		return f
	}
	if err := installJSWithWorkersStopped(site, fw, dir, io.Discard); err != nil {
		t.Fatalf("install: %v", err)
	}

	stop := slices.Index(fake.calls, "stop lerd-vite-myapp")
	install := slices.Index(fake.calls, "install")
	start := slices.Index(fake.calls, "start lerd-vite-myapp")
	if stop < 0 || install < stop || start < install {
		t.Errorf("want vite stopped, then the install, then vite started; calls: %v", fake.calls)
	}
	if slices.Contains(fake.calls, "stop lerd-queue-myapp") {
		t.Errorf("a container worker does not run on node_modules and was stopped; calls: %v", fake.calls)
	}
	if _, err := os.Stat(wrapper); err != nil {
		t.Errorf("generated dev server config missing after the install: %v", err)
	}
}

// Without a site there are no workers to look for: the install just runs.
func TestInstallJSWithoutSiteOnlyInstalls(t *testing.T) {
	ran := false
	prev := installJSDeps
	installJSDeps = func(string, io.Writer) error { ran = true; return nil }
	t.Cleanup(func() { installJSDeps = prev })

	if err := installJSWithWorkersStopped(nil, nil, t.TempDir(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("install did not run")
	}
}
