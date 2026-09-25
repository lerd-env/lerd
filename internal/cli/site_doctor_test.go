package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/sitedoctor"
)

func TestResolveSiteDoctorTarget_DefaultsToCwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path, _, label, err := resolveSiteDoctorTarget("")
	if err != nil {
		t.Fatalf("cwd target: %v", err)
	}
	if path != cwd || label != cwd {
		t.Errorf("expected cwd %q, got path=%q label=%q", cwd, path, label)
	}
}

func TestResolveSiteDoctorTarget_UnknownDomain(t *testing.T) {
	if _, _, _, err := resolveSiteDoctorTarget("does-not-exist.invalid"); err == nil {
		t.Error("expected an error for an unknown domain")
	}
}

func TestResolveSiteDoctorTarget_AcceptsSiteName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := config.AddSite(config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}
	path, _, _, err := resolveSiteDoctorTarget("myapp")
	if err != nil || path != "/srv/myapp" {
		t.Fatalf("site name target: path=%q err=%v", path, err)
	}
}

func TestDoctorGlyph(t *testing.T) {
	cases := map[string]string{
		sitedoctor.StatusOK:      "✓",
		sitedoctor.StatusWarn:    "⚠",
		sitedoctor.StatusFail:    "✗",
		sitedoctor.StatusUnknown: "?",
	}
	for status, glyph := range cases {
		if got := doctorGlyph(status); !strings.Contains(got, glyph) {
			t.Errorf("doctorGlyph(%q)=%q, want it to contain %q", status, got, glyph)
		}
	}
}

// --fix resolves the findings lerd can act on by itself, which today is a
// drifted vhost, and reports on what is left rather than on what it found.
func TestApplySiteDoctorFixes_regeneratesADriftedVhost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: config.CanonicalPath(t.TempDir()), PHPVersion: "8.3"}
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if err := nginx.GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	site.PHPVersion = "8.4"
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	before := sitedoctor.RunForPath(context.Background(), site.Path, "")
	if !hasVhostWarning(before) {
		t.Fatal("the report did not flag the drifted vhost to begin with")
	}

	after := applySiteDoctorFixes(site.Path, "", before, true)
	if hasVhostWarning(after) {
		t.Error("the vhost is still reported as drifted after --fix")
	}
	conf, err := os.ReadFile(filepath.Join(config.NginxConfD(), "myapp.test.conf"))
	if err != nil {
		t.Fatalf("reading the vhost: %v", err)
	}
	if !strings.Contains(string(conf), "lerd-php84-fpm") {
		t.Errorf("vhost was not regenerated:\n%s", conf)
	}
}

func hasVhostWarning(resp sitedoctor.Response) bool {
	for _, c := range resp.Checks {
		if c.Name == "vhost" && c.Status == sitedoctor.StatusWarn {
			return true
		}
	}
	return false
}

// `--json` writes a document to stdout, so a fix that shells out must not let
// the child print there: the report stops being parseable.
func TestFixOutput_keepsTheJSONDocumentAlone(t *testing.T) {
	if got := fixOutput(true); got != os.Stderr {
		t.Errorf("json mode writes subprocess output to %v, want stderr", got)
	}
	if got := fixOutput(false); got != os.Stdout {
		t.Errorf("normal mode writes subprocess output to %v, want stdout", got)
	}
}
