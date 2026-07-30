package reqstats

import "testing"

func TestIsDevServerRequest(t *testing.T) {
	// Everything a dev server serves lives under the prefix lerd gives it, which
	// is how one nginx location carries the modules and the hot-reload socket.
	dev := []string{
		"/@lerd-vite/@vite/client",
		"/@lerd-vite/@react-refresh",
		"/@lerd-vite/src/App.vue",
		"/@lerd-vite/src/main.ts?t=1753875600000",
		"/@lerd-vite/node_modules/.vite/deps/vue.js",
		"/@lerd-vite/",
	}
	for _, u := range dev {
		if !IsDevServerRequest(u) {
			t.Errorf("IsDevServerRequest(%q) = false, want true", u)
		}
	}
	app := []string{
		"/",
		"/account",
		"/api/health",
		"/build/assets/app-DQ0VOLSm.js",
		// The prefix only means a dev server when the path starts with it.
		"/docs/@lerd-vite/getting-started",
		// A route that shares the opening of the prefix is still the app's.
		"/@lerd-viteworks",
	}
	for _, u := range app {
		if IsDevServerRequest(u) {
			t.Errorf("IsDevServerRequest(%q) = true, want false", u)
		}
	}
}
