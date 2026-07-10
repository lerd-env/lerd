package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestDeclaredFalse(t *testing.T) {
	for _, v := range []string{"false", "FALSE", " false "} {
		if !declaredFalse(v) {
			t.Errorf("declaredFalse(%q) = false, want true", v)
		}
	}
	// Empty and "auto" mean "detect", not "opt out".
	for _, v := range []string{"", "auto", "true"} {
		if declaredFalse(v) {
			t.Errorf("declaredFalse(%q) = true, want false", v)
		}
	}
}

// A framework definition declaring `npm: false` (Magento, Drupal, WordPress)
// must not have its JS steps offered, and the same for `composer: false`.
func TestFrameworkOptOutGatesPackageManagers(t *testing.T) {
	magento := &config.Framework{Composer: "auto", NPM: "false"}
	if declaredFalse(magento.Composer) {
		t.Error("composer: auto should not be treated as opted out")
	}
	if !declaredFalse(magento.NPM) {
		t.Error("npm: false should be treated as opted out")
	}

	wordpress := &config.Framework{Composer: "false", NPM: "false"}
	if !declaredFalse(wordpress.Composer) || !declaredFalse(wordpress.NPM) {
		t.Error("wordpress opts out of both")
	}
}

func TestFrameworkForSetupNeverNil(t *testing.T) {
	fw := frameworkForSetup(nil, t.TempDir())
	if fw == nil {
		t.Fatal("frameworkForSetup returned nil")
	}
	// Reading fields on the zero value must not panic.
	if declaredFalse(fw.NPM) || declaredFalse(fw.Composer) {
		t.Error("zero framework should not opt out of anything")
	}
}

// A framework with no env section (Magento keeps config in app/etc/env.php) must
// not be handed to `lerd env`, which would fail and print the error twice.
func TestHasEnvConfig(t *testing.T) {
	var nilFW *config.Framework
	if nilFW.HasEnvConfig() {
		t.Error("nil framework reports env config")
	}
	if (&config.Framework{}).HasEnvConfig() {
		t.Error("empty env section reports env config")
	}
	withFile := &config.Framework{Env: config.FrameworkEnvConf{File: ".env"}}
	if !withFile.HasEnvConfig() {
		t.Error("env.file should count")
	}
	withServices := &config.Framework{Env: config.FrameworkEnvConf{
		Services: map[string]config.FrameworkServiceDef{"mysql": {}},
	}}
	if !withServices.HasEnvConfig() {
		t.Error("env.services should count")
	}
	withFallback := &config.Framework{Env: config.FrameworkEnvConf{FallbackFile: "wp-config.php"}}
	if !withFallback.HasEnvConfig() {
		t.Error("env.fallback_file should count")
	}
}
