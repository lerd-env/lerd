//go:build windows

package cli

import (
	"reflect"
	"testing"
)

func TestAppWindowCommand(t *testing.T) {
	installed := map[string]string{
		`msedge.exe`: `C:\Edge\msedge.exe`,
		`chrome.exe`: `C:\Chrome\chrome.exe`,
		`brave.exe`:  `C:\Brave\brave.exe`,
	}
	find := func(name string) string { return installed[name] }
	url := "http://lerd.localhost"

	cases := []struct {
		name, progID string
		find         func(string) string
		want         []string
	}{
		{"edge first without a default", "", find, []string{`C:\Edge\msedge.exe`, "--app=" + url}},
		{"default chromium browser wins", "ChromeHTML", find, []string{`C:\Chrome\chrome.exe`, "--app=" + url}},
		{"brave default", "BraveHTML", find, []string{`C:\Brave\brave.exe`, "--app=" + url}},
		{"firefox default falls back to edge", "FirefoxURL-308046B0AF4A39CB", find, []string{`C:\Edge\msedge.exe`, "--app=" + url}},
		{"default not installed falls back", "BraveHTML", func(n string) string {
			if n == "chrome.exe" {
				return `C:\Chrome\chrome.exe`
			}
			return ""
		}, []string{`C:\Chrome\chrome.exe`, "--app=" + url}},
		{"no chromium browser", "", func(string) string { return "" }, nil},
	}
	for _, c := range cases {
		if got := appWindowCommand(url, c.progID, c.find); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: appWindowCommand = %v, want %v", c.name, got, c.want)
		}
	}
}
