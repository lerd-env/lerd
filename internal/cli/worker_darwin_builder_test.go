package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/services"
)

// These builder tests are platform-agnostic — they exercise pure string
// generation used on macOS in the `exec` and `container` worker modes.
// We test them here so the logic stays covered on Linux CI runs too.

func TestBuildDarwinExecWorkerService_PointsAtGuardScript(t *testing.T) {
	serviceUnit := buildDarwinExecWorkerService("/run/workers/lerd-queue-alpha.sh", "always")

	if !strings.Contains(serviceUnit, "ExecStart=/bin/sh '/run/workers/lerd-queue-alpha.sh'") {
		t.Errorf("service unit should call guard script via /bin/sh, got:\n%s", serviceUnit)
	}
	if !strings.Contains(serviceUnit, "Restart=always") {
		t.Errorf("service unit missing Restart=always")
	}
	if !strings.Contains(serviceUnit, "WantedBy=default.target") {
		t.Errorf("service unit missing default.target WantedBy")
	}
}

func TestBuildDarwinExecWorkerService_ScriptPathSurvivesTheSplit(t *testing.T) {
	// The launchd translator splits ExecStart into argv, so a guard script
	// under a data dir with a space in it has to come back as one argument.
	script := "/Users/me/My Data/lerd/workers/lerd-queue-alpha.sh"
	unit := buildDarwinExecWorkerService(script, "always")
	line := findLine(unit, "ExecStart=")
	if line == "" {
		t.Fatalf("no ExecStart= line")
	}
	args := services.SplitExecStart(strings.TrimPrefix(line, "ExecStart="))
	want := []string{"/bin/sh", script}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("ExecStart argv = %q, want %q", args, want)
	}
}

func TestBuildDarwinExecWorkerGuardScript_WrapsPodmanExec(t *testing.T) {
	pidFile := "/run/workers/lerd-queue-alpha.pid"
	podmanBin := "/opt/homebrew/bin/podman"
	container := "lerd-php84-fpm"
	sitePath := "/Users/u/alpha"
	workerCmd := "php artisan queue:work"
	runCmd := "/opt/homebrew/bin/podman exec -w /Users/u/alpha lerd-php84-fpm php artisan queue:work"

	script := buildDarwinExecWorkerGuardScript(pidFile, podmanBin, container, sitePath, workerCmd, runCmd)

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Errorf("guard script should start with shebang, got:\n%s", script)
	}
	for _, want := range []string{pidFile, runCmd, container, sitePath, "pgrep -f", "readlink /proc/$p/cwd", "'php artisan queue:work'"} {
		if !strings.Contains(script, want) {
			t.Errorf("guard script missing %q:\n%s", want, script)
		}
	}
}

func TestBuildDarwinContainerWorkerUnit_UsesFPMImage(t *testing.T) {
	unit := buildDarwinContainerWorkerUnit(
		"lerd-queue-alpha",     // unitName
		"8.4",                  // phpVersion
		"/Users/u/alpha",       // sitePath
		"/Users/u/home",        // homeDir
		"/lerd/php.conf",       // phpConfFile
		"/lerd/php-user.ini",   // phpUserIniFile
		"/lerd/php-shared.ini", // phpSharedIniFile
		"php artisan queue:work",
		"always",
		false, // custom container
	)

	for _, want := range []string{
		"Image=lerd-php84-fpm:local",
		"ContainerName=lerd-queue-alpha",
		"WorkingDir=/Users/u/alpha",
		"Exec=php artisan queue:work",
		"Restart=always",
		"Volume=/lerd/php-shared.ini:/usr/local/etc/php/conf.d/95-lerd-shared.ini:ro",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("container unit missing %q:\n%s", want, unit)
		}
	}
}

func TestBuildDarwinContainerWorkerUnit_CustomContainerUsesSiteImage(t *testing.T) {
	unit := buildDarwinContainerWorkerUnit(
		"lerd-custom-alpha",
		"",
		"/Users/u/alpha",
		"/Users/u/home",
		"", "", "",
		"node worker.js",
		"always",
		true, // custom container = true, image comes from caller
	)
	if strings.Contains(unit, "lerd-php") {
		t.Errorf("custom container unit should not reference a PHP FPM image:\n%s", unit)
	}
}

func TestBuildDarwinHostWorkerService_PointsAtGuardScript(t *testing.T) {
	unit := buildDarwinHostWorkerService("/run/workers/lerd-vite-alpha.sh", "always")

	if !strings.Contains(unit, "ExecStart=/bin/sh '/run/workers/lerd-vite-alpha.sh'") {
		t.Errorf("host worker service unit should /bin/sh the guard, got:\n%s", unit)
	}
	if !strings.Contains(unit, "Restart=always") {
		t.Errorf("host worker service unit missing Restart=always")
	}
	if !strings.Contains(unit, "Description=Lerd Worker (host mode)") {
		t.Errorf("host worker service unit missing host-mode description")
	}

	line := findLine(unit, "ExecStart=")
	rhs := strings.TrimPrefix(line, "ExecStart=")
	if fields := strings.Fields(rhs); len(fields) != 2 {
		t.Errorf("ExecStart RHS must split into 2 fields for the launchd translator; got %d: %q", len(fields), rhs)
	}
}

func TestBuildDarwinHostWorkerGuardScript_WrapsFnmExec(t *testing.T) {
	fnm := "/Users/u/.local/share/lerd/bin/fnm"
	binDir := "/Users/u/.local/share/lerd/bin"
	sitePath := "/Users/u/alpha"
	command := "npm run dev"

	script := buildDarwinHostWorkerGuardScript(fnm, binDir, "22", sitePath, command, "")

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Errorf("guard script should start with shebang, got:\n%s", script)
	}
	for _, want := range []string{
		"cd '/Users/u/alpha'",
		"'/Users/u/.local/share/lerd/bin/fnm' exec --using=22",
		"-- /bin/sh -c 'npm run dev'",
		"export PATH=",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("guard script missing %q:\n%s", want, script)
		}
	}
}

func TestBuildDarwinHostWorkerGuardScript_EscapesSingleQuotes(t *testing.T) {
	// A worker command containing a single quote must survive the
	// '"'"' shell-escape idiom we use to keep the surrounding 'sh -c …'
	// quoting intact. Without the escape, the trailing ' would close
	// the sh -c string early and the rest of the command would parse
	// as separate shell tokens.
	script := buildDarwinHostWorkerGuardScript(
		"/bin/fnm", "/Users/u/.local/share/lerd/bin", "22", "/site",
		`node -e 'console.log("x")'`, "",
	)
	if !strings.Contains(script, `'"'"'console.log("x")'"'"'`) {
		t.Errorf("expected escaped single quotes in guard script, got:\n%s", script)
	}
}

// Vite's Inertia/Wayfinder plugin shells out to `php artisan` from inside
// `npm run dev`. lerd's BinDir holds the php/composer/laravel shims, so
// it must lead PATH for the subprocess to find them — issue #375.
func TestBuildDarwinHostWorkerGuardScript_PrependsLerdBinDirToPath(t *testing.T) {
	binDir := "/Users/u/.local/share/lerd/bin"
	script := buildDarwinHostWorkerGuardScript("/bin/fnm", binDir, "22", "/site", "npm run dev", "")
	want := `export PATH="/Users/u/.local/share/lerd/bin:/opt/homebrew/bin:`
	if !strings.Contains(script, want) {
		t.Errorf("guard script must prepend lerd BinDir to PATH; got:\n%s", script)
	}
}

func findLine(body, prefix string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func TestWorkerBuilders_ForceColour(t *testing.T) {
	t.Setenv("NO_COLOR", "") // the assertions below are the colour-on path
	unit := buildDarwinContainerWorkerUnit(
		"lerd-queue-alpha", "8.4", "/Users/u/alpha", "/Users/u/home",
		"/lerd/php.conf", "/lerd/php-user.ini", "/lerd/php-shared.ini",
		"php artisan queue:work", "always", false,
	)
	if !strings.Contains(unit, `Environment="FORCE_COLOR=1"`) {
		t.Errorf("container worker unit should force colour:\n%s", unit)
	}

	custom := buildDarwinContainerWorkerUnit(
		"lerd-custom-alpha", "", "/Users/u/alpha", "/Users/u/home",
		"", "", "", "node worker.js", "always", true,
	)
	if !strings.Contains(custom, `Environment="FORCE_COLOR=1"`) {
		t.Errorf("custom container worker unit should force colour:\n%s", custom)
	}

	guard := buildDarwinHostWorkerGuardScript("/bin/fnm", "/lerd/bin", "22", "/site", "npm run dev", "")
	if !strings.Contains(guard, "export FORCE_COLOR=1") {
		t.Errorf("host worker guard should export the colour vars:\n%s", guard)
	}

	exec := buildWorkerExecCommand("/usr/bin/podman", "/site", "lerd-php84-fpm", "php artisan queue:work", nil)
	if !strings.Contains(exec, "--env=FORCE_COLOR=1") {
		t.Errorf("exec worker command should pass the colour vars:\n%s", exec)
	}
	if !strings.HasSuffix(exec, "lerd-php84-fpm php artisan queue:work") {
		t.Errorf("exec worker command should end with container and command:\n%s", exec)
	}
}

// The worker's own env has to land before the container name, or podman reads
// it as part of the command instead of as a flag.
func TestBuildWorkerExecCommand_EnvArgsPrecedeContainer(t *testing.T) {
	exec := buildWorkerExecCommand("/usr/bin/podman", "/site", "lerd-php84-fpm", "php artisan queue:work",
		[]string{"--env=CHOKIDAR_INTERVAL=2000"})

	if !strings.Contains(exec, "--env=CHOKIDAR_INTERVAL=2000") {
		t.Fatalf("exec worker command should carry the env args:\n%s", exec)
	}
	if strings.Index(exec, "--env=CHOKIDAR_INTERVAL=2000") > strings.Index(exec, "lerd-php84-fpm") {
		t.Errorf("env args must come before the container name:\n%s", exec)
	}
	if !strings.HasSuffix(exec, "lerd-php84-fpm php artisan queue:work") {
		t.Errorf("exec worker command should end with container and command:\n%s", exec)
	}
}

func TestWorkerBuilders_RespectNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	guard := buildDarwinHostWorkerGuardScript("/bin/fnm", "/lerd/bin", "22", "/site", "npm run dev", "")
	if strings.Contains(guard, "FORCE_COLOR") {
		t.Errorf("NO_COLOR should suppress the colour exports:\n%s", guard)
	}
	if got := workerColorArgs(); got != "" {
		t.Errorf("workerColorArgs() = %q, want empty under NO_COLOR", got)
	}
}
