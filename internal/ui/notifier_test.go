package ui

import (
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
			if got := notifySink(tc.cfg, tc.supported); got != tc.want {
				t.Fatalf("notifySink()=%d, want %d", got, tc.want)
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
