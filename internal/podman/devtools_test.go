package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestDevtoolsIni_SubstitutesPlaceholders(t *testing.T) {
	withTempXDG(t)
	ini, err := DevtoolsIni()
	if err != nil {
		t.Fatalf("DevtoolsIni: %v", err)
	}
	if strings.Contains(ini, "{{") {
		t.Errorf("ini still has unsubstituted placeholders: %s", ini)
	}
	if !strings.Contains(ini, "lerd.devtools_host="+config.DevtoolsBridgeTarget()) {
		t.Errorf("ini missing host target: %s", ini)
	}
	if !strings.Contains(ini, "lerd.devtools_kinds=query") {
		t.Errorf("ini missing kinds: %s", ini)
	}
	// Capture shares the debug bridge's sentinel, so the extension reads the
	// same enabled.flag rather than a separate devtools.flag.
	if !strings.Contains(ini, "lerd.devtools_flag=/usr/local/etc/lerd/enabled.flag") {
		t.Errorf("ini flag should point at the shared enabled.flag: %s", ini)
	}
}

func TestEnsureDevtoolsAssets_WritesIni(t *testing.T) {
	withTempXDG(t)
	if err := EnsureDevtoolsAssets(); err != nil {
		t.Fatalf("EnsureDevtoolsAssets: %v", err)
	}
	b, err := os.ReadFile(config.DevtoolsIniFile())
	if err != nil {
		t.Fatalf("read ini: %v", err)
	}
	if !strings.Contains(string(b), "lerd.devtools_host=") {
		t.Errorf("ini content unexpected: %s", string(b))
	}
}

// The Containerfile compiles lerd_devtools in the builder stage and carries a
// marker hashing the source, so any change to the C drifts the image hash and
// triggers a base rebuild + a NeedsFPMRebuild for updating users. If this fails
// after editing the extension, update the `lerd_devtools-src-sha256:` line in
// lerd-php-fpm.Containerfile to the printed value.
func TestDevtoolsSourceMarkerInSync(t *testing.T) {
	want, err := devtoolsSourceHash()
	if err != nil {
		t.Fatalf("devtoolsSourceHash: %v", err)
	}
	tmpl, err := GetQuadletTemplate("lerd-php-fpm.Containerfile")
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	marker := "lerd_devtools-src-sha256: " + want
	if !strings.Contains(tmpl, marker) {
		t.Errorf("Containerfile marker out of date; expected %q. Update the lerd_devtools-src-sha256 line.", marker)
	}
	// The builder must actually compile and enable the extension.
	if !strings.Contains(tmpl, "COPY internal/podman/devtools") || !strings.Contains(tmpl, "docker-php-ext-enable lerd_devtools") {
		t.Error("Containerfile no longer compiles/enables lerd_devtools in the builder stage")
	}
}

func TestWriteDevtoolsSource_StagesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := writeDevtoolsSource(dir); err != nil {
		t.Fatalf("writeDevtoolsSource: %v", err)
	}
	// Staged at the repo-relative path the Containerfile's COPY expects.
	for _, name := range []string{"config.m4", "php_lerd_devtools.h", "lerd_devtools.c"} {
		if _, err := os.Stat(dir + "/internal/podman/devtools/" + name); err != nil {
			t.Errorf("expected staged %s: %v", name, err)
		}
	}
}

// Every collector call goes through lerd_call_collector, which skips the call
// when the collector did not load. Calling a missing function raises an
// "Invalid callback" the application cannot catch, and that turned the first
// request on a Symfony worker into a 500. A new seam that calls the collector
// directly would bring the class of bug straight back.
func TestDevtoolsCollectorCallsAreGuarded(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("devtools", "lerd_devtools.c"))
	if err != nil {
		t.Fatalf("read extension source: %v", err)
	}
	body := string(src)

	// The helper itself is the one place allowed to reach call_user_function.
	if n := strings.Count(body, "call_user_function("); n != 1 {
		t.Errorf("expected exactly one call_user_function (inside lerd_call_collector), found %d", n)
	}
	if !strings.Contains(body, "lerd_collector_has(fn, fn_len)") {
		t.Error("lerd_call_collector no longer checks the function exists before calling it")
	}
	// Loading must latch on the result, not on the attempt, or a failed include
	// leaves every later seam in the request calling a function that is absent.
	if strings.Contains(body, "LERD_G(collector_loaded) = 1;") {
		t.Error("collector load latches before the include is known to have worked")
	}
	if !strings.Contains(body, "LERD_G(collector_loaded) = lerd_collector_has(") {
		t.Error("collector load no longer latches on whether the functions appeared")
	}
	// A missing assets directory is a no-op, the same rule the Laravel adapter
	// already followed.
	if !strings.Contains(body, "lerd_asset(path, sizeof(path), \"devtools-collector.php\");\n\t/* Same rule as the Laravel adapter") {
		t.Error("collector load no longer checks the asset exists before including it")
	}
}
