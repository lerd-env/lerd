package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func browserEventsText(t *testing.T, args map[string]any) map[string]any {
	t.Helper()
	res, _ := execBrowserEvents(args)
	m := res.(map[string]any)
	text := m["content"].([]map[string]any)[0]["text"].(string)
	if m["isError"] == true {
		return map[string]any{"error": text}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("decoding %s: %v", text, err)
	}
	return out
}

func TestBrowserEvents_ExplainsAnEmptyAnswer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubRoundTrip(t, "[]")
	off := false
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), BrowserCapture: &config.BrowserCapture{Enabled: &off}}); err != nil {
		t.Fatal(err)
	}

	siteOff := browserEventsText(t, map[string]any{"site": "shop"})
	if siteOff["site_enabled"] != false || !strings.Contains(siteOff["hint"].(string), "browser_toggle") {
		t.Fatalf("site off = %v", siteOff)
	}
	debugOff := browserEventsText(t, map[string]any{})
	if debugOff["debug_enabled"] != false || !strings.Contains(debugOff["hint"].(string), "dumps_toggle") {
		t.Fatalf("debug off = %v", debugOff)
	}
	cfg, _ := config.LoadGlobal()
	cfg.SetDumpsEnabled(true)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	on := browserEventsText(t, map[string]any{})
	if !strings.Contains(on["hint"].(string), "Load or reload a page") {
		t.Fatalf("on = %v", on)
	}
}

func TestBrowserToggle_NeedsASite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	res, _ := execBrowserCaptureToggle(map[string]any{"enable": true})
	m := res.(map[string]any)
	if m["isError"] != true || !strings.Contains(m["content"].([]map[string]any)[0]["text"].(string), `"site"`) {
		t.Fatalf("got %v", m)
	}
}

func TestBrowserEvents_RefusesAnUnknownType(t *testing.T) {
	stubRoundTrip(t, "[]")
	got := browserEventsText(t, map[string]any{"types": []any{"errors"}})
	if !strings.Contains(got["error"].(string), `"errors"`) {
		t.Fatalf("got %v", got)
	}
}
