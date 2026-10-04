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

	off := browserEventsText(t, map[string]any{})
	if off["enabled"] != false || !strings.Contains(off["hint"].(string), "browser_toggle") {
		t.Fatalf("off = %v", off)
	}
	cfg, _ := config.LoadGlobal()
	cfg.BrowserCapture.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	on := browserEventsText(t, map[string]any{})
	if !strings.Contains(on["hint"].(string), "Load or reload a page") {
		t.Fatalf("on = %v", on)
	}
}

func TestBrowserEvents_RefusesAnUnknownType(t *testing.T) {
	stubRoundTrip(t, "[]")
	got := browserEventsText(t, map[string]any{"types": []any{"errors"}})
	if !strings.Contains(got["error"].(string), `"errors"`) {
		t.Fatalf("got %v", got)
	}
}
