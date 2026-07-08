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
