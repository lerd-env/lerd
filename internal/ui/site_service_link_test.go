package ui

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func writeLerdYAML(t *testing.T, site *config.Site, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(site.Path, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLinkableServicesListsWhatIsInstalledAndUnused(t *testing.T) {
	site, _ := linkTestSite(t, "mailpit", "redis")
	saveService(t, mariadbAlternate())
	saveService(t, &config.CustomService{Name: "phpmyadmin"})
	writeLerdYAML(t, site, "services:\n  - redis\n")

	// redis is used already, phpmyadmin has nothing to wire, mysql is not installed.
	got := linkableServices(site)
	slices.Sort(got)
	if want := []string{"mailpit", "mariadb-12-3"}; !slices.Equal(got, want) {
		t.Errorf("linkable = %v, want %v", got, want)
	}
}

// Two services writing the same address would leave the env on whichever ran
// last, so a second database or a second redis is not on offer.
func TestLinkableServicesLeavesOutATakenAddress(t *testing.T) {
	site, _ := linkTestSite(t, "mysql", "postgres", "redis", "mailpit", "meilisearch")
	saveService(t, mariadbAlternate())
	saveService(t, &config.CustomService{
		Name: "valkey", Preset: "valkey", EnvRole: "redis",
		EnvVars: []string{"REDIS_HOST=lerd-valkey"},
	})
	saveService(t, &config.CustomService{Name: "mongo", Family: "mongo", EnvVars: []string{"MONGO_DSN=mongodb://lerd-mongo:27017"}})
	saveService(t, &config.CustomService{
		Name:    "beanstalkd",
		EnvVars: []string{"QUEUE_CONNECTION=beanstalkd", "BEANSTALKD_QUEUE_HOST=lerd-beanstalkd"},
	})
	writeLerdYAML(t, site, "services:\n  - mariadb-12-3\n  - redis\n")

	got := linkableServices(site)
	slices.Sort(got)
	if want := []string{"beanstalkd", "mailpit", "meilisearch", "mongo"}; !slices.Equal(got, want) {
		t.Errorf("linkable = %v, want %v", got, want)
	}
}

func TestLinkSiteServiceRefusesWhatIsNotOnOffer(t *testing.T) {
	site, ran := linkTestSite(t, "mailpit", "mysql", "postgres")
	writeLerdYAML(t, site, "services:\n  - mysql\n")
	for _, name := range []string{"", "solr", "redis", "mysql", "postgres", "../mailpit"} {
		if err := linkSiteService(site, name); err == nil {
			t.Errorf("%q was accepted", name)
		}
	}
	body, _ := os.ReadFile(filepath.Join(site.Path, ".lerd.yaml"))
	if string(body) != "services:\n  - mysql\n" || len(*ran) != 0 {
		t.Errorf(".lerd.yaml = %q, ran = %v; a refusal must change nothing", body, *ran)
	}
}

// The entry has to mean the same service on a teammate's machine, so a versioned
// alternate is recorded as its preset and version.
func TestLinkSiteServiceRecordsAVersionedAlternate(t *testing.T) {
	site, ran := linkTestSite(t)
	saveService(t, mariadbAlternate())
	if err := linkSiteService(site, "mariadb-12-3"); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Services) != 1 {
		t.Fatalf("services = %+v", proj.Services)
	}
	if s := proj.Services[0]; s.Name != "mariadb-12-3" || s.Preset != "mariadb" || s.PresetVersion != "12.3" || s.Custom != nil {
		t.Errorf("entry = %+v", s)
	}
	if want := []string{"link --yes", "env"}; !slices.Equal(*ran, want) {
		t.Errorf("ran %v, want %v", *ran, want)
	}
}

// An external server's host and user are this machine's, so only its name goes
// into the committed file.
func TestLinkSiteServiceKeepsAnExternalServerOutOfTheFile(t *testing.T) {
	site, _ := linkTestSite(t)
	saveService(t, &config.CustomService{
		Name: "office", Family: "mysql", EnvVars: []string{"DB_HOST=lerd-mysql"},
		External: &config.ExternalDB{Host: "db.example.test", Port: 3306},
	})
	if err := linkSiteService(site, "office"); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(site.Path, ".lerd.yaml"))
	if strings.Contains(string(body), "db.example.test") || !strings.Contains(string(body), "- office") {
		t.Errorf(".lerd.yaml = %s", body)
	}
}

func TestLinkSiteServiceLiftsAnEarlierRemoval(t *testing.T) {
	site, _ := linkTestSite(t, "mailpit")
	if err := config.SetSiteServiceDeclined("shop", "mailpit", true); err != nil {
		t.Fatal(err)
	}
	site, _ = config.FindSite("shop")
	ran := false
	runLerdFn = func(string, ...string) ([]byte, error) {
		ran = true
		if s, _ := config.FindSite("shop"); s.DeclinesService("mailpit") {
			t.Error("lerd ran while mailpit was still declined")
		}
		return nil, nil
	}
	if err := linkSiteService(site, "mailpit"); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("nothing wired the service")
	}
}

func TestLinkSiteServiceReportsAFailedCommand(t *testing.T) {
	site, _ := linkTestSite(t, "mailpit")
	runLerdFn = func(string, ...string) ([]byte, error) { return []byte("boom"), errors.New("exit 1") }
	if err := linkSiteService(site, "mailpit"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want the command's output", err)
	}
}

// A declared database this machine never installed still holds its address, in
// either form the file names it.
func TestLinkableServicesCountsADeclaredServiceThatIsNotInstalled(t *testing.T) {
	for _, entry := range []string{"mysql-8-4", "mysql-8-4:\n      preset: mysql\n      version: \"8.4\""} {
		site, _ := linkTestSite(t, "postgres", "mailpit")
		writeLerdYAML(t, site, "services:\n  - "+entry+"\n")
		if got := linkableServices(site); !slices.Equal(got, []string{"mailpit"}) {
			t.Errorf("%q: linkable = %v, want only mailpit", entry, got)
		}
	}
}

// Older projects carry sqlite in the list, which env would keep wiring over the
// database just added. A service that is no database leaves it there.
func TestLinkSiteServiceReplacesASQLiteEntryOnlyForADatabase(t *testing.T) {
	for name, want := range map[string][]string{"mysql": {"mysql"}, "mailpit": {"mailpit", "sqlite"}} {
		site, _ := linkTestSite(t, "mysql", "mailpit")
		writeLerdYAML(t, site, "services:\n  - sqlite\n")
		if err := linkSiteService(site, name); err != nil {
			t.Fatal(err)
		}
		proj, _ := config.LoadProjectConfig(site.Path)
		if got := proj.ServiceNames(); !slices.Equal(got, want) {
			t.Errorf("after %s: services = %v, want %v", name, got, want)
		}
	}
}

func TestSiteServiceLinkableEndpoint(t *testing.T) {
	site, _ := linkTestSite(t, "mailpit")
	for domain, want := range map[string]string{
		site.Domains[0]: `{"services":["mailpit"]}`,
		"nope.test":     `{"ok":false,"error":"site not found: nope.test"}`,
	} {
		rec := httptest.NewRecorder()
		handleSiteAction(rec, httptest.NewRequest("GET", "/api/sites/"+domain+"/service:linkable", nil))
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != 200 || got != want {
			t.Errorf("%s: %d %s, want %s", domain, rec.Code, got, want)
		}
	}
}
