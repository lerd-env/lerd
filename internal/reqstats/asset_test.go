package reqstats

import "testing"

func TestIsStaticAsset(t *testing.T) {
	assets := []string{
		"/build/assets/AccountNav-DQ0VOLSm.js",
		"/build/assets/Index-BesPMiNk.css",
		"/css/app.css?v=2",
		"/img/logo.png",
		"/favicon.ico",
		"/fonts/inter.woff2",
		"/assets/app-abc123.js.map",
	}
	for _, u := range assets {
		if !IsStaticAsset(u) {
			t.Errorf("IsStaticAsset(%q) = false, want true", u)
		}
	}
	routes := []string{
		"/",
		"/account",
		"/account/profiles/5",
		"/api/health",
		"/users?tab=js",   // query mentioning js, but the path isn't an asset
		"/reports/weekly", // no extension
		"/v2/things",
	}
	for _, u := range routes {
		if IsStaticAsset(u) {
			t.Errorf("IsStaticAsset(%q) = true, want false", u)
		}
	}
}

func TestIsAppRequest(t *testing.T) {
	type req struct {
		what   string
		status int
		uri    string
		ms     float64
	}
	served := []req{
		{"page", 200, "/account", 36},
		{"api", 204, "/api/health", 2.5},
		{"redirect", 302, "/login", 12},
		{"error page", 500, "/checkout", 1480},
	}
	for _, r := range served {
		if !IsAppRequest(r.status, r.uri, r.ms) {
			t.Errorf("IsAppRequest(%s) = false, want true", r.what)
		}
	}
	skipped := []req{
		// An upgraded WebSocket is logged once, at close, with the whole lifetime
		// of the socket as its request time.
		{"websocket upgrade", 101, "/app/cb1dxmnqqfb88d7hnchk", 3623181},
		{"short websocket", 101, "/app/key", 40},
		{"static asset", 200, "/build/assets/app-DQ0VOLSm.js", 4},
		{"file nginx served directly", 200, "/manifest.json", 0},
	}
	for _, r := range skipped {
		if IsAppRequest(r.status, r.uri, r.ms) {
			t.Errorf("IsAppRequest(%s) = true, want false", r.what)
		}
	}
}
