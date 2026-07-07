package ui

import (
	"testing"
	"time"
)

func TestAnalyticsRange(t *testing.T) {
	cases := map[string]struct {
		dur   time.Duration
		label string
	}{
		"15m":  {15 * time.Minute, "15m"},
		"1h":   {time.Hour, "1h"},
		"24h":  {24 * time.Hour, "24h"},
		"7d":   {7 * 24 * time.Hour, "7d"},
		"":     {time.Hour, "1h"}, // absent falls back to 1h
		"nope": {time.Hour, "1h"}, // unknown falls back to 1h
	}
	for in, want := range cases {
		dur, label := analyticsRange(in)
		if dur != want.dur || label != want.label {
			t.Errorf("analyticsRange(%q) = %v/%q, want %v/%q", in, dur, label, want.dur, want.label)
		}
	}
}
