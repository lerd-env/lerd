package podman

import (
	"errors"
	"slices"
	"testing"
)

func TestProcRouteHasDefault(t *testing.T) {
	header := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"
	cases := []struct {
		name  string
		table string
		want  bool
	}{
		{"default via ethernet", header + "enp1s0\t00000000\t017AA8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
			"enp1s0\t007AA8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n", true},
		{"only a link-down docker0 subnet", header + "docker0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n", false},
		{"empty table", header, false},
		{"unreadable", "", false},
	}
	for _, c := range cases {
		if got := procRouteHasDefault(c.table); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// stubNetns fakes the three probes the routeless check reads, and records the
// units the heal stops and starts.
type stubNetns struct {
	up, hostDefault bool
	routes          []string // successive answers to `ip route show default` in the netns
	attached        []string
	stopped         []string
	started         []string
}

func (s *stubNetns) install(t *testing.T) {
	prevUp, prevHost, prevRoutes := rootlessNetnsUpFn, hostHasDefaultRouteFn, netnsDefaultRouteFn
	prevAttached, prevStop, prevStart := runningOnNetworkFn, stopUnitFn, startUnitFn
	t.Cleanup(func() {
		rootlessNetnsUpFn, hostHasDefaultRouteFn, netnsDefaultRouteFn = prevUp, prevHost, prevRoutes
		runningOnNetworkFn, stopUnitFn, startUnitFn = prevAttached, prevStop, prevStart
	})
	rootlessNetnsUpFn = func() bool { return s.up }
	hostHasDefaultRouteFn = func() bool { return s.hostDefault }
	netnsDefaultRouteFn = func() (string, error) {
		r := s.routes[0]
		if len(s.routes) > 1 {
			s.routes = s.routes[1:]
		}
		return r, nil
	}
	runningOnNetworkFn = func(string) ([]string, error) { return s.attached, nil }
	stopUnitFn = func(n string) error { s.stopped = append(s.stopped, n); return nil }
	startUnitFn = func(n string) error { s.started = append(s.started, n); return nil }
}

func TestRootlessNetnsLacksDefaultRoute(t *testing.T) {
	cases := []struct {
		name string
		s    stubNetns
		want bool
	}{
		{"netns templated on docker0 while the host is online", stubNetns{up: true, hostDefault: true, routes: []string{""}}, true},
		{"healthy netns", stubNetns{up: true, hostDefault: true, routes: []string{"default via 192.168.122.1 dev enp1s0"}}, false},
		{"host offline too, a rebuild would change nothing", stubNetns{up: true, hostDefault: false, routes: []string{""}}, false},
		{"no netns running", stubNetns{up: false, hostDefault: true, routes: []string{""}}, false},
	}
	for _, c := range cases {
		c.s.install(t)
		if got := RootlessNetnsLacksDefaultRoute(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHealRoutelessNetns_restartsTheAttachedContainers(t *testing.T) {
	s := stubNetns{up: true, hostDefault: true, routes: []string{"", "default via 192.168.122.1 dev enp1s0"},
		attached: []string{"lerd-nginx", "lerd-php84-fpm"}}
	s.install(t)

	healed, err := HealRoutelessNetns()
	if err != nil || !healed {
		t.Fatalf("healed=%v err=%v", healed, err)
	}
	if !slices.Equal(s.stopped, s.attached) || !slices.Equal(s.started, s.attached) {
		t.Errorf("stopped %v, started %v, want both %v", s.stopped, s.started, s.attached)
	}
}

func TestHealRoutelessNetns_healthyNetnsIsLeftAlone(t *testing.T) {
	s := stubNetns{up: true, hostDefault: true, routes: []string{"default via 192.168.122.1 dev enp1s0"},
		attached: []string{"lerd-nginx"}}
	s.install(t)

	if healed, err := HealRoutelessNetns(); healed || err != nil {
		t.Fatalf("healed=%v err=%v", healed, err)
	}
	if len(s.stopped) != 0 {
		t.Errorf("stopped %v on a healthy netns", s.stopped)
	}
}

func TestHealRoutelessNetns_reportsAContainerHoldingTheNetns(t *testing.T) {
	s := stubNetns{up: true, hostDefault: true, routes: []string{""}, attached: []string{"lerd-nginx"}}
	s.install(t)

	healed, err := HealRoutelessNetns()
	if healed || !errors.Is(err, ErrNetnsHeld) {
		t.Fatalf("healed=%v err=%v, want ErrNetnsHeld", healed, err)
	}
	if !slices.Equal(s.started, s.attached) {
		t.Errorf("started %v, want the stopped containers back %v", s.started, s.attached)
	}
}
