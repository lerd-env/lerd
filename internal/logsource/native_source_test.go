package logsource

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// `lerd logs` follows the same target as the dashboard's log tab. Under the
// native runtime the container is stopped by design, so reading it shows a
// frozen log from before the switch instead of what is serving now.
func TestFPMContainerFollowsTheNativeRuntime(t *testing.T) {
	site := &config.Site{Name: "shop", PHPVersion: "8.4", Path: t.TempDir()}

	if got := fpmTarget(site, "8.4", false); got != "lerd-php84-fpm" {
		t.Errorf("container mode = %q, want lerd-php84-fpm", got)
	}
	if got := fpmTarget(site, "8.4", true); got != "lerd-native-php84" {
		t.Errorf("native mode = %q, want lerd-native-php84", got)
	}
}

// Sites the native runtime never touches keep reading their own container.
func TestFPMContainerLeavesOwnContainerSitesAlone(t *testing.T) {
	for _, s := range []*config.Site{
		{Name: "shop", Runtime: "frankenphp", Path: t.TempDir()},
		{Name: "shop", ContainerPort: 8080, Path: t.TempDir()},
	} {
		if got := fpmTarget(s, "8.4", true); got == "lerd-native-php84" {
			t.Errorf("%+v must not read the native log", s)
		}
	}
}
