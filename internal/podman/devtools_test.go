package podman

import (
	"os"
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

func TestDevtoolsBuildBlock_Shape(t *testing.T) {
	b := devtoolsBuildBlock()
	if !strings.Contains(b, "COPY lerd-devtools /tmp/lerd-devtools") {
		t.Errorf("block missing COPY: %s", b)
	}
	if !strings.Contains(b, "docker-php-ext-enable lerd_devtools") {
		t.Errorf("block missing enable: %s", b)
	}
	if !strings.Contains(b, "|| true") {
		t.Errorf("block must degrade gracefully on build failure: %s", b)
	}
	// Layered onto a toolchain-less image, so it adds and removes the build
	// deps in the same RUN to avoid bloating the layer.
	if !strings.Contains(b, "apk add --no-cache --virtual .lerd-build") || !strings.Contains(b, "apk del .lerd-build") {
		t.Errorf("block must add and remove the toolchain: %s", b)
	}
}

func TestWriteDevtoolsSource_StagesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := writeDevtoolsSource(dir); err != nil {
		t.Fatalf("writeDevtoolsSource: %v", err)
	}
	for _, name := range []string{"config.m4", "php_lerd_devtools.h", "lerd_devtools.c"} {
		if _, err := os.Stat(dir + "/lerd-devtools/" + name); err != nil {
			t.Errorf("expected staged %s: %v", name, err)
		}
	}
}
