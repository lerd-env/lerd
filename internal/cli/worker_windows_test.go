//go:build windows

package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
)

func TestWindowsExecWorkerRunsAtTheMachinePath(t *testing.T) {
	lerdBin := `C:\Users\me\AppData\Local\lerd\bin\lerd.exe`
	podmanBin := `C:\Program Files\RedHat\Podman\podman.exe`
	unit := buildWindowsExecWorkerUnit("lerd-queue-shop", "Queue Worker", "shop", `C:\Sites\my shop`, lerdBin, podmanBin, "lerd-php85-fpm", "php artisan queue:work --tries=3", "always")

	if !strings.Contains(unit, "Restart=always\n") {
		t.Errorf("unit does not carry the restart policy:\n%s", unit)
	}
	got := services.SplitExecStart(execStartLine(unit))
	if len(got) < 14 {
		t.Fatalf("ExecStart argv too short: %q", got)
	}
	// The worker runs through the guard, named by its unit.
	if !reflect.DeepEqual(got[:5], []string{lerdBin, "worker-exec", "--unit", "lerd-queue-shop", "--"}) {
		t.Errorf("argv starts %q, want lerd worker-exec --unit <unit> --", got[:5])
	}
	// The podman path keeps its space and the site runs at its /mnt path.
	if got[5] != podmanBin || got[6] != "exec" || got[7] != "-w" || got[8] != "/mnt/c/Sites/my shop" {
		t.Errorf("argv continues %q, want podman exec -w at the machine path", got[5:9])
	}
	if !reflect.DeepEqual(got[len(got)-5:], []string{"lerd-php85-fpm", "php", "artisan", "queue:work", "--tries=3"}) {
		t.Errorf("argv ends %q, want the container then the worker command", got[len(got)-5:])
	}
	if !strings.Contains(strings.Join(got, " "), "--env=LERD_SITE=shop") {
		t.Errorf("argv %q does not name the site", got)
	}
}

func TestWindowsContainerWorkerMountsTheSite(t *testing.T) {
	unit := buildWindowsContainerWorkerUnit("lerd-queue-shop", "lerd-php85-fpm:local", `C:\Sites\shop`, "php artisan queue:work", "always", true, "8.5")
	for _, want := range []string{
		"Image=lerd-php85-fpm:local\n",
		"ContainerName=lerd-queue-shop\n",
		`Volume=C:\Sites\shop:C:\Sites\shop:rw` + "\n",
		`WorkingDir=C:\Sites\shop` + "\n",
		"Volume=" + config.ContainerHostsFile() + ":/etc/hosts:ro\n",
		"Volume=" + config.SharedIniFile() + ":/usr/local/etc/php/conf.d/95-lerd-shared.ini:ro\n",
		"Exec=php artisan queue:work\n",
		"Restart=always\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("container unit lacks %q:\n%s", want, unit)
		}
	}

	custom := buildWindowsContainerWorkerUnit("lerd-queue-shop", "lerd-custom-shop:local", `C:\Sites\shop`, "php artisan queue:work", "always", false, "")
	if strings.Contains(custom, "conf.d") {
		t.Errorf("a custom container worker should not get lerd's PHP config:\n%s", custom)
	}
}

func TestWindowsWorkerSupport(t *testing.T) {
	if ok, _ := workerSupportedOnPlatform(config.FrameworkWorker{Command: "php artisan queue:work"}); !ok {
		t.Error("a container worker should run on Windows")
	}
	if ok, reason := workerSupportedOnPlatform(config.FrameworkWorker{Command: "npm run dev", Host: true}); ok || reason == "" {
		t.Error("a host worker should be refused with a reason")
	}
	if ok, reason := workerSupportedOnPlatform(config.FrameworkWorker{Command: "php artisan schedule:run", Schedule: "minutely"}); ok || reason == "" {
		t.Error("a scheduled worker should be refused with a reason")
	}
}

func TestWindowsWorkerStopReapsTheContainerSide(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var reaped []workerReap
	prev := reapWorkerInContainer
	reapWorkerInContainer = func(r workerReap) { reaped = append(reaped, r) }
	t.Cleanup(func() { reapWorkerInContainer = prev })

	want := workerReap{Container: "lerd-php85-fpm", Command: "php artisan queue:work", Dir: "/mnt/c/Sites/shop"}
	saveWorkerReap("lerd-queue-shop", want)
	removeWorkerExecArtifacts("lerd-queue-shop")

	if len(reaped) != 1 || reaped[0] != want {
		t.Errorf("reaped %v, want %v", reaped, want)
	}
	if _, err := os.Stat(workerReapPath("lerd-queue-shop")); !os.IsNotExist(err) {
		t.Error("the sidecar should be gone after the stop")
	}

	removeWorkerExecArtifacts("lerd-queue-shop")
	if len(reaped) != 1 {
		t.Error("a second stop with no sidecar should reap nothing")
	}
}

func TestWindowsWorkerExecClearsLeftoversBeforeRunning(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var reaped []workerReap
	prev := reapWorkerInContainer
	reapWorkerInContainer = func(r workerReap) { reaped = append(reaped, r) }
	t.Cleanup(func() { reapWorkerInContainer = prev })

	want := workerReap{Container: "lerd-php85-fpm", Command: "php artisan queue:work", Dir: "/mnt/c/Sites/shop"}
	saveWorkerReap("lerd-queue-shop", want)

	code, err := runWorkerExec("lerd-queue-shop", []string{"cmd", "/c", "exit 3"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want the worker's 3", code)
	}
	if len(reaped) != 1 || reaped[0] != want {
		t.Errorf("reaped %v before running, want %v", reaped, want)
	}
	if _, err := os.Stat(workerReapPath("lerd-queue-shop")); err != nil {
		t.Error("a start must keep the sidecar for the stop that follows")
	}
}
