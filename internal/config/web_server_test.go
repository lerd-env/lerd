package config

import "testing"

func TestWebServerKind(t *testing.T) {
	cases := []struct {
		value, kind string
		known       bool
	}{
		{"", WebServerNginx, true},
		{"nginx", WebServerNginx, true},
		{"go", WebServerGo, true},
		// A value a newer lerd wrote falls back to nginx, the server every
		// binary knows, and is reported as unknown so the caller can say so.
		{"caddy", WebServerNginx, false},
	}
	for _, c := range cases {
		kind, known := (&GlobalConfig{WebServer: c.value}).WebServerKind()
		if kind != c.kind || known != c.known {
			t.Errorf("%q: got (%q, %v), want (%q, %v)", c.value, kind, known, c.kind, c.known)
		}
	}
}

func TestValidateWebServer(t *testing.T) {
	for _, ok := range []string{"nginx", "go"} {
		if err := ValidateWebServer(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	if err := ValidateWebServer("apache"); err == nil {
		t.Error("apache accepted")
	}
}
