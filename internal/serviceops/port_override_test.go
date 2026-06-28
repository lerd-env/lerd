package serviceops

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

func TestWithURLPort(t *testing.T) {
	cases := []struct {
		in   string
		port int
		want string
	}{
		{"mysql://root:lerd@127.0.0.1:3306/lerd", 3307, "mysql://root:lerd@127.0.0.1:3307/lerd"},
		{"", 3307, ""},
		{"mysql://root:lerd@127.0.0.1:3306/lerd", 0, "mysql://root:lerd@127.0.0.1:3306/lerd"},
	}
	for _, c := range cases {
		if got := WithURLPort(c.in, c.port); got != c.want {
			t.Errorf("WithURLPort(%q, %d) = %q, want %q", c.in, c.port, got, c.want)
		}
	}
}

// TestMysqlPresetPortOverride validates the override against the real mysql
// preset: the canonical version publishes 3306, and moving the primary host
// port + connection URL to 3307 keeps the container-internal port at 3306.
func TestMysqlPresetPortOverride(t *testing.T) {
	p, err := config.LoadPreset("mysql")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := p.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.Ports) == 0 || svc.Ports[0] != "3306:3306" {
		t.Fatalf("preset primary port = %v, want first entry 3306:3306", svc.Ports)
	}

	moved := podman.SetPrimaryHostPort(svc.Ports, 3307)
	if moved[0] != "3307:3306" {
		t.Errorf("moved primary port = %q, want 3307:3306 (container-internal port unchanged)", moved[0])
	}
	if url := WithURLPort(svc.ConnectionURL, 3307); !strings.Contains(url, ":3307/") {
		t.Errorf("connection URL after move = %q, want host port 3307", url)
	}
}
