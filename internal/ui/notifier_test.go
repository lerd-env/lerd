package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/geodro/lerd/internal/push"
)

func cfgWith(disabled bool, target string) *config.GlobalConfig {
	c := &config.GlobalConfig{}
	c.Notifications.Disabled = disabled
	c.Notifications.Target = target
	return c
}

func TestNotifySink(t *testing.T) {
	cases := []struct {
		name      string
		cfg       *config.GlobalConfig
		supported bool
		want      sink
	}{
		{"nil config is off", nil, true, sinkOff},
		{"globally disabled is off", cfgWith(true, config.NotifyTargetNative), true, sinkOff},
		{"unset target is browser", cfgWith(false, ""), true, sinkBrowser},
		{"native and supported", cfgWith(false, config.NotifyTargetNative), true, sinkNative},
		{"native but unsupported falls back to browser", cfgWith(false, config.NotifyTargetNative), false, sinkBrowser},
		{"explicit browser", cfgWith(false, config.NotifyTargetBrowser), true, sinkBrowser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := notifySink(tc.cfg, func() bool { return tc.supported }); got != tc.want {
				t.Fatalf("notifySink()=%d, want %d", got, tc.want)
			}
		})
	}
}

// The probe talks to D-Bus, so it must not run for a sink that will never
// deliver natively — notifications off, or the browser target.
func TestNotifySinkProbesOnlyForNativeTarget(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.GlobalConfig
	}{
		{"nil config", nil},
		{"globally disabled", cfgWith(true, config.NotifyTargetNative)},
		{"browser target", cfgWith(false, config.NotifyTargetBrowser)},
		{"unset target", cfgWith(false, "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probed := false
			notifySink(tc.cfg, func() bool { probed = true; return true })
			if probed {
				t.Fatal("native support probe ran for a non-native sink")
			}
		})
	}
}

func TestNativeRequest(t *testing.T) {
	got := nativeRequest(push.Notification{
		Title:   "Migrate finished: myshop",
		Body:    "Took 2.3s.",
		Urgency: "critical",
	})
	if got.AppName != notifyAppName {
		t.Errorf("AppName=%q, want %q", got.AppName, notifyAppName)
	}
	if got.Summary != "Migrate finished: myshop" || got.Body != "Took 2.3s." {
		t.Errorf("summary/body not carried through: %+v", got)
	}
	if got.Urgency != desktopnotify.UrgencyCritical {
		t.Errorf("Urgency=%d, want critical", got.Urgency)
	}
}

// The desktop sink must reach the package seam rather than the notification
// daemon itself: a suite that emits for real raises popups on the desktop of
// whoever is running it, naming the temp directories of the tests that fired.
func TestDispatchNotification_NativeSinkGoesThroughTheSeam(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfgDir := config.ConfigDir()
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("notifications:\n  target: native\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var got desktopnotify.Request
	prev := emitDesktopNotification
	prevSupported := desktopSupported
	emitDesktopNotification = func(r desktopnotify.Request) (uint32, error) {
		got = r
		return 0, nil
	}
	// A CI runner has no notification daemon, so the live probe would send this
	// down the browser branch and the test would pass or fail on the machine
	// rather than on the code.
	desktopSupported = func() bool { return true }
	t.Cleanup(func() {
		emitDesktopNotification = prev
		desktopSupported = prevSupported
	})

	dispatchNotification(push.Notification{Kind: "test", Title: "Create finished: myapp", Body: "Took 0s."})

	if got.Summary != "Create finished: myapp" {
		t.Errorf("the desktop notification did not go through the seam, got %+v", got)
	}
}

// TestDispatchNotification_MessageReachesTheDesktopWhileFocused covers the one
// kind the focus rule does not apply to. A focused dashboard is assumed to be
// showing what the notification would say, which is not true of a message: it
// went to a real person, off a machine that cannot take it back, and a
// developer on another tab has no sign of it.
func TestDispatchNotification_MessageReachesTheDesktopWhileFocused(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfgDir := config.ConfigDir()
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("notifications:\n  target: native\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var got []string
	prev := emitDesktopNotification
	prevSupported := desktopSupported
	emitDesktopNotification = func(r desktopnotify.Request) (uint32, error) {
		got = append(got, r.Summary)
		return 0, nil
	}
	desktopSupported = func() bool { return true }
	focusedClients.Store(1)
	t.Cleanup(func() {
		emitDesktopNotification = prev
		desktopSupported = prevSupported
		focusedClients.Store(0)
	})

	dispatchNotification(push.Notification{Kind: "mail", Title: "New email"})
	dispatchNotification(push.Notification{Kind: "message", Title: "Message sent from acme"})

	if len(got) != 1 || got[0] != "Message sent from acme" {
		t.Errorf("raised %v, want the message alone: mail waits in the bell while the dashboard has focus", got)
	}
}
