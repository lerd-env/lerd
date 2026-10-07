package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubFrankenPHPRestore records which FrankenPHP units a restore writes, with
// installed reporting which unit files already exist.
func stubFrankenPHPRestore(t *testing.T, installed map[string]bool) *[]string {
	t.Helper()
	var wrote []string
	prevInstalled, prevWrite := frankenPHPUnitInstalled, writeFrankenPHPQuadlet
	t.Cleanup(func() { frankenPHPUnitInstalled, writeFrankenPHPQuadlet = prevInstalled, prevWrite })
	frankenPHPUnitInstalled = func(name string) bool { return installed[name] }
	writeFrankenPHPQuadlet = func(site, _, _ string, _ []string, _ map[string]string) error {
		wrote = append(wrote, site)
		return nil
	}
	return &wrote
}

// A sites:restore or a reinstall leaves a FrankenPHP site registered with no
// unit file; lerd start has to write it, or nginx proxies to nothing (502).
func TestRestoreFrankenPHPQuadlet_writesAMissingUnit(t *testing.T) {
	wrote := stubFrankenPHPRestore(t, nil)

	restoreFrankenPHPQuadlet(config.Site{Name: "demo", Path: "/srv/demo", PHPVersion: "8.5", Runtime: "frankenphp"})

	if len(*wrote) != 1 || (*wrote)[0] != "demo" {
		t.Fatalf("wrote %v, want the demo unit", *wrote)
	}
}

func TestRestoreFrankenPHPQuadlet_leavesAnInstalledUnitAlone(t *testing.T) {
	wrote := stubFrankenPHPRestore(t, map[string]bool{"lerd-fp-demo": true})

	restoreFrankenPHPQuadlet(config.Site{Name: "demo", Path: "/srv/demo", PHPVersion: "8.5", Runtime: "frankenphp"})

	if len(*wrote) != 0 {
		t.Fatalf("rewrote %v, want an installed unit left alone", *wrote)
	}
}

func TestRestoreFrankenPHPQuadlet_skipsFPMSites(t *testing.T) {
	wrote := stubFrankenPHPRestore(t, nil)

	restoreFrankenPHPQuadlet(config.Site{Name: "demo", Path: "/srv/demo", PHPVersion: "8.5"})

	if len(*wrote) != 0 {
		t.Fatalf("wrote %v for an FPM site", *wrote)
	}
}
