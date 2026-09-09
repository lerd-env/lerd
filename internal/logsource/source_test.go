package logsource

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

var fpmRe = regexp.MustCompile(`^lerd-php\d+-fpm$`)

// seedSite points the data dir at a temp location and registers a site whose
// project dir declares two workers.
func seedSite(t *testing.T) (siteName, sitePath string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath = t.TempDir()
	yaml := "workers:\n  - queue\n  - horizon\n"
	if err := os.WriteFile(filepath.Join(sitePath, ".lerd.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write .lerd.yaml: %v", err)
	}
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: sitePath, PHPVersion: "8.4"}
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	return "myapp", sitePath
}

func sourceByName(srcs []Source, name string) (Source, bool) {
	for _, s := range srcs {
		if s.Name == name {
			return s, true
		}
	}
	return Source{}, false
}

func TestSources_EnumeratesSiteAndGlobals(t *testing.T) {
	// These assert the container-runtime target, so pin the runtime rather than
	// reading whatever the developer's own install happens to be set to.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name, path := seedSite(t)
	srcs, err := Sources(name, path)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}

	want := map[string]struct {
		kind  Kind
		scope Scope
	}{
		"fpm":            {KindPodman, ScopeSite},
		"worker:queue":   {KindJournal, ScopeSite},
		"worker:horizon": {KindJournal, ScopeSite},
		"nginx":          {KindPodman, ScopeGlobal},
		"dns":            {KindPodman, ScopeGlobal},
		"watcher":        {KindJournal, ScopeGlobal},
		"ui":             {KindJournal, ScopeGlobal},
	}
	for n, exp := range want {
		s, ok := sourceByName(srcs, n)
		if !ok {
			t.Errorf("missing source %q", n)
			continue
		}
		if s.Kind != exp.kind {
			t.Errorf("%s kind = %v, want %v", n, s.Kind, exp.kind)
		}
		if s.Scope != exp.scope {
			t.Errorf("%s scope = %v, want %v", n, s.Scope, exp.scope)
		}
	}

	// The FPM container follows the detected/registered PHP version; assert the
	// shape rather than a fixed version, which varies by machine default.
	if fpm, _ := sourceByName(srcs, "fpm"); !fpmRe.MatchString(fpm.Locator) {
		t.Errorf("fpm locator = %q, want lerd-php<NN>-fpm", fpm.Locator)
	}
	if w, _ := sourceByName(srcs, "worker:queue"); w.Locator != "lerd-queue-myapp" {
		t.Errorf("worker locator = %q, want lerd-queue-myapp", w.Locator)
	}
}

func TestSources_NoSiteStillReturnsGlobals(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srcs, err := Sources("", "")
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if _, ok := sourceByName(srcs, "nginx"); !ok {
		t.Error("expected global nginx source even without a site")
	}
	if _, ok := sourceByName(srcs, "fpm"); ok {
		t.Error("did not expect a site fpm source without a site")
	}
}

func TestResolve_UnknownListsValidNames(t *testing.T) {
	name, path := seedSite(t)
	_, err := Resolve(name, path, "bogus")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
	if !strings.Contains(err.Error(), "nginx") {
		t.Errorf("error should list valid names, got %q", err.Error())
	}
}

func TestResolve_FindsKnownSource(t *testing.T) {
	name, path := seedSite(t)
	s, err := Resolve(name, path, "worker:horizon")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if s.Locator != "lerd-horizon-myapp" {
		t.Errorf("locator = %q, want lerd-horizon-myapp", s.Locator)
	}
}

func TestFPMContainer_SiteTypes(t *testing.T) {
	// These assert the container-runtime target, so pin the runtime rather than
	// reading whatever the developer's own install happens to be set to.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cases := []struct {
		name string
		site config.Site
		want string
	}{
		{"custom-container", config.Site{Name: "nestapp", ContainerPort: 3000}, "lerd-custom-nestapp"},
		{"frankenphp", config.Site{Name: "fapp", Runtime: "frankenphp"}, "lerd-fp-fapp"},
		{"custom-fpm", config.Site{Name: "cfapp", Runtime: "fpm-custom"}, "lerd-cfpm-cfapp"},
		{"host-proxy", config.Site{Name: "hp", HostPort: 5173}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.site
			if got := FPMContainer(&s); got != tc.want {
				t.Errorf("FPMContainer = %q, want %q", got, tc.want)
			}
		})
	}
	// A normal site falls back to the shared per-version container.
	plain := config.Site{Name: "plain", PHPVersion: "8.4", Path: t.TempDir()}
	if got := FPMContainer(&plain); !fpmRe.MatchString(got) {
		t.Errorf("normal FPMContainer = %q, want lerd-php<NN>-fpm", got)
	}
}

func TestResolve_DirectSources(t *testing.T) {
	// These assert the container-runtime target, so pin the runtime rather than
	// reading whatever the developer's own install happens to be set to.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name, path := seedSite(t)
	cases := []struct{ in, wantLocator string }{
		{"fpm", ""}, // matched by pattern below
		{"worker:queue", "lerd-queue-myapp"},
		{"nginx", "lerd-nginx"},
		{"dns", "lerd-dns"},
		{"php8.4", "lerd-php84-fpm"},
	}
	for _, tc := range cases {
		s, err := Resolve(name, path, tc.in)
		if err != nil {
			t.Errorf("Resolve(%q): %v", tc.in, err)
			continue
		}
		if tc.in == "fpm" {
			if !fpmRe.MatchString(s.Locator) {
				t.Errorf("fpm locator = %q, want lerd-php<NN>-fpm", s.Locator)
			}
			continue
		}
		if s.Locator != tc.wantLocator {
			t.Errorf("Resolve(%q) locator = %q, want %q", tc.in, s.Locator, tc.wantLocator)
		}
	}
}

func TestResolve_UndeclaredWorkerIsUnknown(t *testing.T) {
	name, path := seedSite(t)
	if _, err := Resolve(name, path, "worker:bogus"); err == nil {
		t.Error("expected undeclared worker to be unknown")
	}
}
