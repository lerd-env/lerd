package config

import "testing"

func TestNotificationTarget(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"unset resolves to browser", "", NotifyTargetBrowser},
		{"native", NotifyTargetNative, NotifyTargetNative},
		{"browser", NotifyTargetBrowser, NotifyTargetBrowser},
		{"garbage falls back to browser", "carrier-pigeon", NotifyTargetBrowser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &GlobalConfig{}
			c.Notifications.Target = tc.value
			if got := c.NotificationTarget(); got != tc.want {
				t.Fatalf("NotificationTarget()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetNotificationTarget(t *testing.T) {
	c := &GlobalConfig{}
	c.SetNotificationTarget(NotifyTargetNative)
	if got := c.NotificationTarget(); got != NotifyTargetNative {
		t.Fatalf("after SetNotificationTarget(native), got %q", got)
	}
}
