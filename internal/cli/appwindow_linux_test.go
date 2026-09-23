//go:build linux

package cli

import (
	"slices"
	"testing"
)

// installed fakes the lookup: only the names given resolve, each to a path
// under /fake so a test can tell which one was picked.
func installed(names ...string) func(string) string {
	return func(name string) string {
		if slices.Contains(names, name) {
			return "/fake/" + name
		}
		return ""
	}
}

func TestAppWindowCommand(t *testing.T) {
	const url = "http://lerd.localhost/"
	cases := []struct {
		name           string
		defaultBrowser string
		find           func(string) string
		want           []string
		brand          string
	}{
		{"native chromium", "firefox.desktop", installed("chromium"),
			[]string{"/fake/chromium", "--app=" + url}, "chrome"},
		{"flatpak export when no binary is on PATH", "", installed("com.brave.Browser"),
			[]string{"/fake/com.brave.Browser", "--app=" + url}, "brave"},
		{"default browser wins over list order", "brave-browser.desktop", installed("chromium", "brave"),
			[]string{"/fake/brave", "--app=" + url}, "brave"},
		{"flatpak default browser wins too", "com.google.Chrome.desktop", installed("chromium", "com.google.Chrome"),
			[]string{"/fake/com.google.Chrome", "--app=" + url}, "chrome"},
		{"default browser not installed falls through", "vivaldi-stable.desktop", installed("chromium"),
			[]string{"/fake/chromium", "--app=" + url}, "chrome"},
		{"no chromium browser", "firefox.desktop", installed("firefox"), nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, brand := appWindowCommand(url, c.defaultBrowser, c.find)
			if !slices.Equal(got, c.want) || brand != c.brand {
				t.Errorf("got %q %q, want %q %q", got, brand, c.want, c.brand)
			}
		})
	}
}

func TestAppWindowKey(t *testing.T) {
	cases := map[string]string{
		"http://lerd.localhost/": "lerd.localhost__",
		"http://127.0.0.1:7073/": "127.0.0.1__",
		"https://lerd.localhost": "lerd.localhost__",
	}
	for url, want := range cases {
		if got := appWindowKey(url); got != want {
			t.Errorf("appWindowKey(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestHyprlandAppWindow(t *testing.T) {
	clients := []byte(`[
		{"address":"0xa","class":"chromium"},
		{"address":"0xb","class":"chrome-lerd.localhost__-Default"},
		{"address":"0xc","class":"brave-lerd.localhost__-Profile_1"}
	]`)
	if got := hyprlandAppWindow(clients, "lerd.localhost__"); got != "0xb" {
		t.Errorf("got %q, want 0xb", got)
	}
	if got := hyprlandAppWindow(clients, "127.0.0.1__"); got != "" {
		t.Errorf("got %q for a window that is not open", got)
	}
	if got := hyprlandAppWindow([]byte("not json"), "lerd.localhost__"); got != "" {
		t.Errorf("got %q from unreadable output", got)
	}
}
