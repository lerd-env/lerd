package cli

import (
	"regexp"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// The applet has to die without taking `lerd tray off` (or the shell that ran
// it) with it, and both command lines contain the words "lerd tray".
func TestTrayProcessPatterns_MatchTheAppletOnly(t *testing.T) {
	applets := []string{
		"/home/u/.local/bin/lerd tray",
		"/home/u/.local/bin/lerd tray --mono",
		"/home/u/.local/bin/lerd-tray",
		"/home/u/.local/bin/lerd tray --mono=false",
		"/home/u/.local/bin/lerd-tray --mono=false",
		"/home/u/.local/bin/lerd-tray --mono",
	}
	others := []string{
		"/home/u/.local/bin/lerd tray off",
		"/home/u/.local/bin/lerd tray on",
		"/home/u/.local/bin/lerd tray icon high-contrast",
		"systemctl --user is-enabled lerd-tray.service",
	}
	var res []*regexp.Regexp
	for _, p := range trayProcessPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("compiling %q: %v", p, err)
		}
		res = append(res, re)
	}
	matches := func(cmdline string) bool {
		for _, re := range res {
			if re.MatchString(cmdline) {
				return true
			}
		}
		return false
	}
	for _, cmdline := range applets {
		if !matches(cmdline) {
			t.Errorf("a running applet must be killable: %q matched nothing", cmdline)
		}
	}
	for _, cmdline := range others {
		if matches(cmdline) {
			t.Errorf("%q is not the applet and must survive killTray", cmdline)
		}
	}
}

func TestRunTrayIconSet_PersistsHighContrast(t *testing.T) {
	withTempXDG(t)

	if err := runTrayIconSet(true); err != nil {
		t.Fatalf("runTrayIconSet(true): %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsHighContrastTrayIcon() {
		t.Error("high-contrast icon preference not persisted")
	}
}

func TestRunTrayIconSet_DefaultRoundTrip(t *testing.T) {
	withTempXDG(t)

	_ = runTrayIconSet(true)
	if err := runTrayIconSet(false); err != nil {
		t.Fatalf("runTrayIconSet(false): %v", err)
	}
	cfg, _ := config.LoadGlobal()
	if cfg.IsHighContrastTrayIcon() {
		t.Error("preference should be back to default after setting default")
	}
}

func TestNewTrayCmd_HasSubcommands(t *testing.T) {
	want := map[string]bool{"icon": false, "on": false, "off": false}
	for _, c := range NewTrayCmd().Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tray command is missing the %s subcommand", name)
		}
	}
}

func TestSetTrayPreference_PersistsOffAndBackOn(t *testing.T) {
	withTempXDG(t)

	changed, err := setTrayPreference(false)
	if err != nil || !changed {
		t.Fatalf("setTrayPreference(false) = %v, %v; want true, nil", changed, err)
	}
	if trayEnabled() {
		t.Fatal("start and install should see the tray as off once it is turned off")
	}
	if changed, _ := setTrayPreference(false); changed {
		t.Error("turning the tray off twice should report no change, so the CLI can say so")
	}
	if changed, err := setTrayPreference(true); err != nil || !changed {
		t.Fatalf("setTrayPreference(true) = %v, %v; want true, nil", changed, err)
	}
	if !trayEnabled() {
		t.Error("turning the tray back on should be visible to start and install")
	}
}

func TestTrayEnabled_DefaultsOn(t *testing.T) {
	withTempXDG(t)

	if !trayEnabled() {
		t.Error("an install that never touched the setting must keep its tray")
	}
}

// A replacement started while the old applet still holds the lock exits at
// once, and the restart ends with no tray at all.
func TestWaitTrayGone_ReturnsOnceTheOldAppletExits(t *testing.T) {
	polls := 0
	running := func() bool { polls++; return polls < 3 }
	if !waitTrayGone(running, time.Second) {
		t.Fatal("the old applet exited, so the wait must report it gone")
	}
	if polls != 3 {
		t.Errorf("polled %d times, want 3: the wait must last until the applet is gone", polls)
	}
}

func TestWaitTrayGone_GivesUpOnAnAppletThatWillNotDie(t *testing.T) {
	if waitTrayGone(func() bool { return true }, 100*time.Millisecond) {
		t.Error("an applet still running at the deadline must not be reported gone")
	}
}
