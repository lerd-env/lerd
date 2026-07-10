package cli

import (
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestPHPIniArgsSortedAndPaired(t *testing.T) {
	fw := &config.Framework{PHP: config.FrameworkPHP{CLIIni: map[string]string{
		"memory_limit": "-1",
		"error_log":    "/tmp/php.log",
	}}}
	got := phpIniArgsFor(fw)
	want := []string{"-d", "error_log=/tmp/php.log", "-d", "memory_limit=-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPHPIniArgsEmpty(t *testing.T) {
	if got := phpIniArgsFor(nil); got != nil {
		t.Fatalf("nil framework: %v", got)
	}
	if got := phpIniArgsFor(&config.Framework{}); got != nil {
		t.Fatalf("no ini: %v", got)
	}
}

// An unsafe value must drop the whole set rather than smuggle a second -d.
func TestPHPIniArgsRejectsUnsafe(t *testing.T) {
	fw := &config.Framework{PHP: config.FrameworkPHP{CLIIni: map[string]string{
		"memory_limit": "-1 -d auto_prepend_file=/tmp/pwn.php",
	}}}
	if got := phpIniArgsFor(fw); got != nil {
		t.Fatalf("unsafe ini rendered: %v", got)
	}
}

// Injected args go first, and PHP lets a later -d override an earlier one, so a
// user's explicit -d still wins.
func TestPrependPHPIniArgs(t *testing.T) {
	ini := []string{"-d", "memory_limit=-1"}
	got := prependPHPIniArgs(ini, []string{"bin/magento", "setup:upgrade"})
	want := []string{"-d", "memory_limit=-1", "bin/magento", "setup:upgrade"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}

	// A user's own -d lands after ours and therefore wins.
	got = prependPHPIniArgs(ini, []string{"-d", "memory_limit=64M", "-r", "echo 1;"})
	want = []string{"-d", "memory_limit=-1", "-d", "memory_limit=64M", "-r", "echo 1;"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}

	// Nothing to inject: args are untouched, and the slice is not aliased.
	orig := []string{"-v"}
	if got := prependPHPIniArgs(nil, orig); !reflect.DeepEqual(got, orig) {
		t.Fatalf("got %v", got)
	}
}

// The setup runner execs `php ...` in the container directly, bypassing the
// shim, so it needs its own injection. Only a php invocation is rewritten.
func TestInjectPHPIniIntoCommand(t *testing.T) {
	ini := []string{"-d", "memory_limit=-1"}
	cases := []struct{ in, want string }{
		{"php bin/magento setup:upgrade", "php -d memory_limit=-1 bin/magento setup:upgrade"},
		{"php -d memory_limit=64M bin/magento x", "php -d memory_limit=-1 -d memory_limit=64M bin/magento x"},
		{"bin/cake migrations migrate", "bin/cake migrations migrate"},
		{"drush cr", "drush cr"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := injectPHPIniIntoCommand(tc.in, ini); got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
	// No ini declared: the command is untouched.
	if got := injectPHPIniIntoCommand("php artisan migrate", nil); got != "php artisan migrate" {
		t.Errorf("got %q", got)
	}
}

// Workers exec their command from a systemd unit, and Magento cannot even
// bootstrap at PHP's 128M default, so a worker that gets no directives simply
// crash-loops. A host worker (npm run dev) is not a php invocation and is left be.
func TestInjectPHPIniIntoWorkerCommands(t *testing.T) {
	ini := []string{"-d", "memory_limit=2G"}
	cases := []struct{ in, want string }{
		{"php bin/magento cron:run", "php -d memory_limit=2G bin/magento cron:run"},
		{"php bin/magento queue:consumers:start async.operations.all",
			"php -d memory_limit=2G bin/magento queue:consumers:start async.operations.all"},
		{"php artisan queue:work --tries=3", "php -d memory_limit=2G artisan queue:work --tries=3"},
		{"npm run dev", "npm run dev"},
		{"bin/console messenger:consume async", "bin/console messenger:consume async"},
	}
	for _, tc := range cases {
		if got := injectPHPIniIntoCommand(tc.in, ini); got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
}
