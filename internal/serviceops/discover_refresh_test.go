package serviceops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

func TestRefreshDiscoverFamilyConsumers_fillsEmptyHostsAfterBulkStart(t *testing.T) {
	withServiceHome(t)
	stubDaemonReload(t)
	prevWait := waitReadyFn
	waitReadyFn = func(string, time.Duration) error { return nil }
	t.Cleanup(func() { waitReadyFn = prevWait })

	prevRun := config.ServiceRunning
	config.ServiceRunning = func(name string) bool { return name == "mariadb-11-8" }
	t.Cleanup(func() { config.ServiceRunning = prevRun })

	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}
	pma := &config.CustomService{
		Name:  "phpmyadmin",
		Image: "docker.io/library/phpmyadmin:latest",
		Ports: []string{"127.0.0.1:8080:80"},
		DynamicEnv: map[string]string{
			"PMA_HOSTS": "discover_family:mysql,mariadb",
		},
	}
	if err := config.SaveCustomService(pma); err != nil {
		t.Fatal(err)
	}

	// Simulate pre-start reconcile: no engine running yet → empty PMA_HOSTS.
	config.ServiceRunning = func(string) bool { return false }
	if err := EnsureCustomServiceQuadlet(pma); err != nil {
		t.Fatalf("seed empty hosts: %v", err)
	}
	empty := readQuadlet(t, "lerd-phpmyadmin")
	if strings.Contains(empty, "lerd-mariadb-11-8") {
		t.Fatalf("seed quadlet should not list mariadb yet:\n%s", empty)
	}

	// Engines are up after bulk start; refresh must rewrite the consumer.
	config.ServiceRunning = func(name string) bool { return name == "mariadb-11-8" }
	RefreshDiscoverFamilyConsumers()

	got := readQuadlet(t, "lerd-phpmyadmin")
	if !strings.Contains(got, "lerd-mariadb-11-8") {
		t.Fatalf("RefreshDiscoverFamilyConsumers should set PMA_HOSTS to running mariadb:\n%s", got)
	}
}

func TestRefreshDiscoverFamilyConsumers_picksUpExpandFamilyOnlyConsumer(t *testing.T) {
	withServiceHome(t)
	stubDaemonReload(t)
	prevWait := waitReadyFn
	waitReadyFn = func(string, time.Duration) error { return nil }
	t.Cleanup(func() { waitReadyFn = prevWait })

	prevRun := config.ServiceRunning
	config.ServiceRunning = func(string) bool { return false }
	t.Cleanup(func() { config.ServiceRunning = prevRun })

	if err := config.SaveCustomService(&config.CustomService{
		Name: "valkey", Image: "x", Family: "valkey", EnvRole: "redis", Preset: "valkey",
	}); err != nil {
		t.Fatal(err)
	}
	ri := &config.CustomService{
		Name:  "redisinsight",
		Image: "docker.io/redis/redisinsight:latest",
		Ports: []string{"127.0.0.1:8085:5540"},
		DynamicEnv: map[string]string{
			"RI_REDIS_HOST": "expand_family:redis,valkey={host}",
		},
	}
	if err := config.SaveCustomService(ri); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCustomServiceQuadlet(ri); err != nil {
		t.Fatalf("seed quadlet: %v", err)
	}

	config.ServiceRunning = func(name string) bool { return name == "valkey" }
	RefreshDiscoverFamilyConsumers()

	got := readQuadlet(t, "lerd-redisinsight")
	if !strings.Contains(got, "RI_REDIS_HOST_1=lerd-valkey") {
		t.Fatalf("expand_family-only consumer should be refreshed with the running valkey:\n%s", got)
	}
}

func TestRegenerateFamilyConsumers_skipsBounceWhenUnchanged(t *testing.T) {
	withServiceHome(t)
	stubDaemonReload(t)
	prevRun := config.ServiceRunning
	config.ServiceRunning = func(name string) bool { return name == "mariadb-11-8" }
	t.Cleanup(func() { config.ServiceRunning = prevRun })

	if err := config.SaveCustomService(&config.CustomService{
		Name: "mariadb-11-8", Image: "x", Family: "mariadb", EnvRole: "mysql", Preset: "mariadb",
	}); err != nil {
		t.Fatal(err)
	}
	pma := &config.CustomService{
		Name:  "phpmyadmin",
		Image: "docker.io/library/phpmyadmin:latest",
		Ports: []string{"127.0.0.1:8080:80"},
		DynamicEnv: map[string]string{
			"PMA_HOSTS": "discover_family:mysql,mariadb",
		},
	}
	if err := config.SaveCustomService(pma); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCustomServiceQuadlet(pma); err != nil {
		t.Fatal(err)
	}
	before := readQuadlet(t, "lerd-phpmyadmin")

	RegenerateFamilyConsumers("mariadb")
	after := readQuadlet(t, "lerd-phpmyadmin")
	if before != after {
		t.Fatal("unchanged host list must not rewrite the quadlet")
	}
}

func stubDaemonReload(t *testing.T) {
	t.Helper()
	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
}

func readQuadlet(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.QuadletDir(), name+".container"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
