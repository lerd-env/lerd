package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
)

func queueSiteName(cwd string) (string, error) {
	reg, err := config.LoadSites()
	if err != nil {
		return "", err
	}
	for _, s := range reg.Sites {
		if s.Path == cwd {
			return s.Name, nil
		}
	}
	// Fall back to directory name.
	name, _ := siteops.SiteNameAndDomain(filepath.Base(cwd), "test")
	return name, nil
}

// queueStartTuned adapts the MCP queue_start tool, which offers the three knobs
// Laravel's queue worker takes, to the generic tuned start. The names match the
// placeholders a framework's tune_command declares; a definition that declares
// fewer (CodeIgniter has no per-job timeout) ignores the rest.
func queueStartTuned(siteName, sitePath, phpVersion, queue string, tries, timeout int) error {
	return StartFrameworkWorkerTuned(siteName, sitePath, phpVersion, "queue", map[string]string{
		"queue":   queue,
		"tries":   strconv.Itoa(tries),
		"timeout": strconv.Itoa(timeout),
	})
}

// QueueRestartForSite gracefully restarts the queue worker by running the
// framework's RestartCommand in the FPM container. No-op when the site has no
// queue unit or the framework declares no restart command (e.g. CodeIgniter).
func QueueRestartForSite(siteName, sitePath, phpVersion string) error {
	unitFile := filepath.Join(config.SystemdUserDir(), "lerd-queue-"+siteName+".service")
	if _, err := os.Stat(unitFile); os.IsNotExist(err) {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(siteFrameworkName(siteName), sitePath)
	if !ok {
		return nil
	}
	worker, ok := fw.Workers["queue"]
	if !ok || worker.RestartCommand == "" {
		return nil
	}
	// Heal legacy units: a graceful restart exits cleanly (code 0), which
	// Restart=on-failure would not respawn. Upgrade them to Restart=always.
	if data, err := os.ReadFile(unitFile); err == nil {
		if healed := strings.ReplaceAll(string(data), "Restart=on-failure", "Restart=always"); healed != string(data) {
			if err := os.WriteFile(unitFile, []byte(healed), 0644); err == nil {
				_ = podman.DaemonReloadFn()
			}
		}
	}
	if phpVersion == "" {
		cfg, _ := config.LoadGlobal()
		phpVersion = cfg.PHP.DefaultVersion
	}
	container := resolveWorkerFPMUnit(siteName, phpVersion)
	if container == "" {
		container = "lerd-php" + strings.ReplaceAll(phpVersion, ".", "") + "-fpm"
	}
	if running, _ := podman.ContainerRunning(container); !running {
		return nil
	}
	args := append([]string{"exec", "-w", sitePath, container}, strings.Fields(worker.RestartCommand)...)
	if _, err := podman.Run(args...); err != nil {
		return fmt.Errorf("queue restart for %s: %w", siteName, err)
	}
	fmt.Printf("Queue worker signaled to restart for %s\n", siteName)
	return nil
}
