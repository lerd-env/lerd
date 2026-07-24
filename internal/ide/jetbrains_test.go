package ide

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func project(t *testing.T, withIdea bool) string {
	t.Helper()
	dir := t.TempDir()
	if withIdea {
		if err := os.MkdirAll(filepath.Join(dir, ".idea"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func read(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".idea", "dataSources.xml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var pg = DataSource{
	Key: "shop", Name: "shop (lerd)", Driver: "postgresql", Class: "org.postgresql.Driver",
	URL: "jdbc:postgresql://127.0.0.1:5433/shop", User: "postgres",
}

// sync drives the one-source case the older tests were written around.
func sync(dir string, ds DataSource) (Outcome, error) {
	out, err := Sync(dir, []DataSource{ds})
	if len(out) == 0 {
		return NotJetBrains, err
	}
	return out[0], err
}

// The .idea directory existing is what says the project is open in a JetBrains
// IDE. lerd creating one for a project that is not would be litter.
func TestSyncSkipsAProjectWithoutIdea(t *testing.T) {
	dir := project(t, false)
	done, err := sync(dir, pg)
	if err != nil {
		t.Fatal(err)
	}
	if done != NotJetBrains {
		t.Error("wrote a data source into a project that is not a JetBrains one")
	}
	if _, err := os.Stat(filepath.Join(dir, ".idea")); !os.IsNotExist(err) {
		t.Error(".idea was created")
	}
}

func TestSyncWritesAWholeFileWhenThereIsNone(t *testing.T) {
	dir := project(t, true)
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	out := read(t, dir)
	for _, want := range []string{
		`<component name="DataSourceManagerImpl"`,
		`name="shop (lerd)"`,
		"<driver-ref>postgresql</driver-ref>",
		"jdbc:postgresql://127.0.0.1:5433/shop",
		"</project>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

const existing = `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="DataSourceManagerImpl" format="xml" multifile-model="true">
    <data-source source="LOCAL" name="production" uuid="0d073425-0b90-47fa-ad3d-859060bb86ac">
      <driver-ref>postgresql</driver-ref>
      <jdbc-url>jdbc:postgresql://db.example.com:5432/main</jdbc-url>
    </data-source>
  </component>
</project>
`

// A user's own sources live in the same file, so lerd owns exactly one entry
// and leaves every byte of the rest alone.
func TestSyncLeavesOtherDataSourcesUntouched(t *testing.T) {
	dir := project(t, true)
	path := filepath.Join(dir, ".idea", "dataSources.xml")
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	out := read(t, dir)
	if !strings.Contains(out, `name="production"`) || !strings.Contains(out, "db.example.com") {
		t.Fatalf("the user's own data source was lost:\n%s", out)
	}
	if !strings.Contains(out, `name="shop (lerd)"`) {
		t.Fatalf("lerd's entry was not added:\n%s", out)
	}
	if strings.Count(out, "<data-source ") != 2 {
		t.Errorf("expected two data sources, got:\n%s", out)
	}
}

// Run twice, the entry is updated in place rather than accumulating, and a port
// that moved is picked up.
func TestSyncUpdatesItsOwnEntryInPlace(t *testing.T) {
	dir := project(t, true)
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	moved := pg
	moved.URL = "jdbc:postgresql://127.0.0.1:5544/shop"
	if _, err := sync(dir, moved); err != nil {
		t.Fatal(err)
	}
	out := read(t, dir)
	if strings.Count(out, "<data-source ") != 1 {
		t.Errorf("entry duplicated:\n%s", out)
	}
	if strings.Contains(out, "5433") || !strings.Contains(out, "5544") {
		t.Errorf("the moved port was not picked up:\n%s", out)
	}
}

func TestRemoveDropsOnlyLerdsEntry(t *testing.T) {
	dir := project(t, true)
	path := filepath.Join(dir, ".idea", "dataSources.xml")
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	out := read(t, dir)
	if strings.Contains(out, "(lerd)") {
		t.Errorf("lerd's entry survived:\n%s", out)
	}
	if !strings.Contains(out, `name="production"`) {
		t.Errorf("the user's own data source was removed:\n%s", out)
	}
}

// The identifier has to be stable across runs, or every sync would add a second
// entry, and distinct per project so two sites never fight over one.
func TestDataSourceUUIDIsStableAndPerProject(t *testing.T) {
	a, b := dataSourceUUID("/home/u/shop", "db"), dataSourceUUID("/home/u/shop", "db")
	if a != b {
		t.Errorf("uuid is not stable: %q vs %q", a, b)
	}
	if a == dataSourceUUID("/home/u/other", "db") {
		t.Error("two projects share one uuid")
	}
	if len(a) != 36 || strings.Count(a, "-") != 4 {
		t.Errorf("not a uuid: %q", a)
	}
}

// A name carrying XML metacharacters must not break the file.
func TestSyncEscapesTheName(t *testing.T) {
	dir := project(t, true)
	ds := pg
	ds.Name = `a & b "c"`
	if _, err := sync(dir, ds); err != nil {
		t.Fatal(err)
	}
	out := read(t, dir)
	if strings.Contains(out, `name="a & b "c""`) {
		t.Errorf("name not escaped:\n%s", out)
	}
	if !strings.Contains(out, "&amp;") {
		t.Errorf("ampersand not escaped:\n%s", out)
	}
}

// The file is a user's to read, so the entry lands indented like its siblings
// and the tag it was inserted above keeps its own indentation.
func TestSyncKeepsTheFileTidy(t *testing.T) {
	dir := project(t, true)
	path := filepath.Join(dir, ".idea", "dataSources.xml")
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	out := read(t, dir)
	if !strings.Contains(out, "\n    <data-source source=\"LOCAL\" name=\"shop (lerd)\"") {
		t.Errorf("entry is not indented with its siblings:\n%s", out)
	}
	if !strings.Contains(out, "\n  </component>") {
		t.Errorf("the component close lost its indentation:\n%s", out)
	}
}

const handWired = `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="DataSourceManagerImpl" format="xml" multifile-model="true">
    <data-source source="LOCAL" name="postgres:lerd-postgres" uuid="0d073425-0b90-47fa-ad3d-859060bb86ac">
      <driver-ref>postgresql</driver-ref>
      <jdbc-url>jdbc:postgresql://localhost:5433/shop</jdbc-url>
    </data-source>
  </component>
</project>
`

// Someone who already wired the connection by hand does not want a second entry
// beside it pointing at the same database, however lerd would have spelled it.
func TestSyncSkipsWhenAnEquivalentConnectionExists(t *testing.T) {
	dir := project(t, true)
	path := filepath.Join(dir, ".idea", "dataSources.xml")
	if err := os.WriteFile(path, []byte(handWired), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err := sync(dir, pg)
	if err != nil {
		t.Fatal(err)
	}
	if wrote != AlreadyConfigured {
		t.Error("added a duplicate of the user's own data source")
	}
	if out := read(t, dir); strings.Contains(out, "(lerd)") {
		t.Errorf("file was modified:\n%s", out)
	}
}

// A hand-wired entry pointing somewhere else is not the same connection, so
// lerd still adds its own.
func TestSyncStillWritesWhenTheExistingOnePointsElsewhere(t *testing.T) {
	dir := project(t, true)
	path := filepath.Join(dir, ".idea", "dataSources.xml")
	stale := strings.Replace(handWired, "localhost:5433/shop", "localhost:5432/other", 1)
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err := sync(dir, pg)
	if err != nil {
		t.Fatal(err)
	}
	if wrote != Written {
		t.Error("a different database should not count as the same connection")
	}
}

// Once lerd owns an entry it keeps it current, even if a matching hand-wired
// one appears later, or the two would fight over every run.
func TestSyncKeepsUpdatingItsOwnEntryDespiteALookalike(t *testing.T) {
	dir := project(t, true)
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	body := read(t, dir)
	body = strings.Replace(body, "</component>",
		"  <data-source source=\"LOCAL\" name=\"mine\" uuid=\"aaaaaaaa-0000-4000-8000-000000000000\">\n"+
			"      <jdbc-url>jdbc:postgresql://localhost:5433/shop</jdbc-url>\n    </data-source>\n  </component>", 1)
	if err := os.WriteFile(filepath.Join(dir, ".idea", "dataSources.xml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	moved := pg
	moved.URL = "jdbc:postgresql://127.0.0.1:5544/shop"
	if wrote, err := sync(dir, moved); err != nil || wrote != Written {
		t.Fatalf("wrote=%v err=%v", wrote, err)
	}
	if out := read(t, dir); !strings.Contains(out, "5544") {
		t.Errorf("lerd's own entry stopped being updated:\n%s", out)
	}
}

func TestSameTarget(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"jdbc:postgresql://localhost:5433/shop", "jdbc:postgresql://127.0.0.1:5433/shop?user=postgres", true},
		{"jdbc:mysql://127.0.0.1:3306/shop", "jdbc:mysql://localhost:3306/shop", true},
		{"jdbc:postgresql://localhost:5433/shop", "jdbc:postgresql://localhost:5432/shop", false},
		{"jdbc:postgresql://localhost:5433/shop", "jdbc:postgresql://localhost:5433/other", false},
		{"jdbc:postgresql://localhost:5433/shop", "jdbc:mysql://localhost:5433/shop", false},
		{"jdbc:redis://lerd-redis:6379/0", "jdbc:postgresql://localhost:5433/shop", false},
	}
	for _, c := range cases {
		if got := sameTarget(c.a, c.b); got != c.want {
			t.Errorf("sameTarget(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// JetBrains authenticates with the user from its own sidecar, not from the URL,
// so an entry without one connects as nobody and comes back empty.
func TestSyncWritesTheUserIntoTheCredentialsSidecar(t *testing.T) {
	dir := project(t, true)
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".idea", "dataSources.local.xml"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{
		`<component name="dataSourceStorageLocal">`,
		"<user-name>postgres</user-name>",
		"<secret-storage>master_key</secret-storage>",
		`name="shop (lerd)"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// The sidecar is the IDE's own bookkeeping for every other data source, so
// lerd's entry goes in beside them without disturbing any.
func TestSyncLeavesTheSidecarsOtherEntriesAlone(t *testing.T) {
	dir := project(t, true)
	local := `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="dataSourceStorageLocal" created-in="PS-262.8665.325">
    <data-source name="production" uuid="0d073425-0b90-47fa-ad3d-859060bb86ac">
      <database-info product="PostgreSQL" version="18.4" />
      <user-name>deploy</user-name>
    </data-source>
  </component>
</project>
`
	path := filepath.Join(dir, ".idea", "dataSources.local.xml")
	if err := os.WriteFile(path, []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	if !strings.Contains(out, "<user-name>deploy</user-name>") || !strings.Contains(out, `created-in="PS-262.8665.325"`) {
		t.Errorf("the IDE's own bookkeeping was disturbed:\n%s", out)
	}
	if strings.Count(out, "<data-source ") != 2 {
		t.Errorf("expected two entries:\n%s", out)
	}
}

func TestRemoveClearsBothFiles(t *testing.T) {
	dir := project(t, true)
	if _, err := sync(dir, pg); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dataSources.xml", "dataSources.local.xml"} {
		b, _ := os.ReadFile(filepath.Join(dir, ".idea", name))
		if strings.Contains(string(b), "(lerd)") {
			t.Errorf("%s still holds lerd's entry:\n%s", name, b)
		}
	}
}

// Sync takes the whole set lerd owns, so two databases keep separate entries
// rather than overwriting each other.
func TestSyncWritesOneEntryPerDatabase(t *testing.T) {
	dir := project(t, true)
	staging := DataSource{Key: "shop_staging", Name: "shop_staging (lerd)", Driver: "postgresql",
		Class: "org.postgresql.Driver", URL: "jdbc:postgresql://127.0.0.1:5433/shop_staging", User: "postgres"}
	out, err := Sync(dir, []DataSource{pg, staging})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != Written || out[1] != Written {
		t.Fatalf("outcomes = %v", out)
	}
	body := read(t, dir)
	if strings.Count(body, "<data-source ") != 2 {
		t.Errorf("expected two entries:\n%s", body)
	}
	for _, want := range []string{`name="shop (lerd)"`, `name="shop_staging (lerd)"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s:\n%s", want, body)
		}
	}
}

// A database that is no longer the project's, because grouping moved the site
// onto the group's shared one, takes its connection with it, without lerd
// having to remember what it wrote last time.
func TestSyncDropsAnEntryWhoseDatabaseIsGone(t *testing.T) {
	dir := project(t, true)
	staging := DataSource{Key: "shop_staging", Name: "shop_staging (lerd)", Driver: "postgresql",
		Class: "org.postgresql.Driver", URL: "jdbc:postgresql://127.0.0.1:5433/shop_staging", User: "postgres"}
	if _, err := Sync(dir, []DataSource{pg, staging}); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, []DataSource{pg}); err != nil {
		t.Fatal(err)
	}
	body := read(t, dir)
	if strings.Contains(body, "shop_staging") {
		t.Errorf("the connection for a database no longer in the set survived:\n%s", body)
	}
	if !strings.Contains(body, `name="shop (lerd)"`) {
		t.Errorf("the site's own connection was lost:\n%s", body)
	}
	local, _ := os.ReadFile(filepath.Join(dir, ".idea", "dataSources.local.xml"))
	if strings.Contains(string(local), "shop_staging") {
		t.Errorf("the sidecar kept the removed entry:\n%s", local)
	}
}

// Cleaning up lerd's own entries must never reach a user's, whatever they are
// called.
func TestSyncCleanupLeavesUserEntriesAlone(t *testing.T) {
	dir := project(t, true)
	path := filepath.Join(dir, ".idea", "dataSources.xml")
	mine := strings.Replace(existing, `name="production"`, `name="@lerd"`, 1)
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, []DataSource{pg}); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, []DataSource{pg}); err != nil {
		t.Fatal(err)
	}
	body := read(t, dir)
	if !strings.Contains(body, `name="@lerd"`) {
		t.Errorf("a user entry whose name merely mentions lerd was removed:\n%s", body)
	}
}

func TestOwns(t *testing.T) {
	for name, want := range map[string]bool{
		"shop (lerd)": true, "@lerd": false, "postgres@lerd": false,
		"lerd": false, "my db (lerd) copy": false,
	} {
		if got := Owns(name); got != want {
			t.Errorf("Owns(%q) = %v, want %v", name, got, want)
		}
	}
}
