package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A framework definition declaring `npm: false` (Magento, Drupal, WordPress)
// must not have its JS steps offered, and the same for `composer: false`.
func TestFrameworkOptOutGatesPackageManagers(t *testing.T) {
	magento := &config.Framework{Composer: "auto", NPM: "false"}
	if !packageManagerEnabled(magento.Composer, "composer") {
		t.Error("composer: auto should not be treated as opted out")
	}
	if packageManagerEnabled(magento.NPM, "npm") {
		t.Error("npm: false should be treated as opted out")
	}

	wordpress := &config.Framework{Composer: "false", NPM: "false"}
	if packageManagerEnabled(wordpress.Composer, "composer") || packageManagerEnabled(wordpress.NPM, "npm") {
		t.Error("wordpress opts out of both")
	}
}

func TestFrameworkForSetupNeverNil(t *testing.T) {
	fw := frameworkForSetup(nil, t.TempDir())
	if fw == nil {
		t.Fatal("frameworkForSetup returned nil")
	}
	// Reading fields on the zero value must not panic.
	if !packageManagerEnabled(fw.NPM, "npm") || !packageManagerEnabled(fw.Composer, "composer") {
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

// A service preset's env_vars are dotenv keys. When the framework already maps
// that service (Magento wires opensearch through dotted php-array paths), the
// preset's keys must not be layered on top, or env.php grows a meaningless
// top-level OPENSEARCH_HOST beside the keys the framework just wrote.
func TestFrameworkMapsService(t *testing.T) {
	fw := &config.Framework{Env: config.FrameworkEnvConf{
		Services: map[string]config.FrameworkServiceDef{
			"mysql":      {Vars: []string{"db.connection.default.host=lerd-mysql"}},
			"opensearch": {Vars: []string{"system.default.catalog.search.engine=opensearch"}},
		},
	}}
	for _, name := range []string{"mysql", "opensearch"} {
		if !frameworkMapsService(fw, name) {
			t.Errorf("framework maps %q but frameworkMapsService said no", name)
		}
	}
	if frameworkMapsService(fw, "mailpit") {
		t.Error("mailpit is not mapped by this framework")
	}
	if frameworkMapsService(nil, "mysql") {
		t.Error("nil framework maps nothing")
	}
	if frameworkMapsService(&config.Framework{}, "mysql") {
		t.Error("framework with no env section maps nothing")
	}
}

// A package-manager state added to the store after this binary shipped must not
// be read as "yes" just because it is not the literal string "false". Unknown
// states fall back to auto-detection and say so, rather than silently running
// composer install against a definition that asked for something else.
func TestPackageManagerState_UnknownIsNotSilentlyEnabled(t *testing.T) {
	for _, v := range []string{"", "auto", "true"} {
		if state := packageManagerState(v); state != pkgManagerAuto {
			t.Errorf("packageManagerState(%q) = %v, want auto", v, state)
		}
	}
	if state := packageManagerState("false"); state != pkgManagerOff {
		t.Errorf("packageManagerState(\"false\") = %v, want off", state)
	}
	if state := packageManagerState("prompt"); state != pkgManagerUnknown {
		t.Errorf("packageManagerState(\"prompt\") = %v, want unknown", state)
	}
}
