package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// ── Logs field on built-in Laravel ───────────────────────────────────────────

func TestLaravelBuiltinHasLogs(t *testing.T) {
	if len(laravelFramework.Logs) == 0 {
		t.Fatal("built-in Laravel should have Logs configured")
	}
	if laravelFramework.Logs[0].Path != "storage/logs/*.log" {
		t.Errorf("expected storage/logs/*.log, got %s", laravelFramework.Logs[0].Path)
	}
	if laravelFramework.Logs[0].Format != "monolog" {
		t.Errorf("expected monolog format, got %s", laravelFramework.Logs[0].Format)
	}
}

func TestGetFrameworkLaravel_BuiltinLogs(t *testing.T) {
	setConfigDir(t)

	fw, ok := GetFramework("laravel")
	if !ok {
		t.Fatal("expected to find laravel framework")
	}
	if len(fw.Logs) == 0 {
		t.Fatal("GetFramework(laravel) should include built-in Logs")
	}
	if fw.Logs[0].Format != "monolog" {
		t.Errorf("expected monolog, got %s", fw.Logs[0].Format)
	}
}

func TestGetFrameworkLaravel_UserOverridesLogs(t *testing.T) {
	setConfigDir(t)

	// Write a user laravel.yaml that overrides logs
	dir := FrameworksDir()
	os.MkdirAll(dir, 0755)

	userFw := Framework{
		Name: "laravel",
		Logs: []FrameworkLogSource{
			{Path: "storage/logs/*.log", Format: "monolog"},
			{Path: "storage/logs/custom/*.log", Format: "monolog"},
		},
	}
	data, _ := yaml.Marshal(userFw)
	os.WriteFile(filepath.Join(dir, "laravel.yaml"), data, 0644)

	fw, ok := GetFramework("laravel")
	if !ok {
		t.Fatal("expected to find laravel")
	}
	if len(fw.Logs) != 2 {
		t.Fatalf("expected 2 log sources from user override, got %d", len(fw.Logs))
	}
	if fw.Logs[1].Path != "storage/logs/custom/*.log" {
		t.Errorf("second log source path = %q", fw.Logs[1].Path)
	}
}

func TestGetFrameworkLaravel_NoUserOverrideKeepsBuiltinLogs(t *testing.T) {
	setConfigDir(t)

	// Write a user laravel.yaml with only workers, no logs
	dir := FrameworksDir()
	os.MkdirAll(dir, 0755)

	userFw := Framework{
		Name: "laravel",
		Workers: map[string]FrameworkWorker{
			"horizon": {Label: "Horizon", Command: "php artisan horizon"},
		},
	}
	data, _ := yaml.Marshal(userFw)
	os.WriteFile(filepath.Join(dir, "laravel.yaml"), data, 0644)

	fw, ok := GetFramework("laravel")
	if !ok {
		t.Fatal("expected to find laravel")
	}
	// Built-in logs should remain since user didn't override
	if len(fw.Logs) != 1 {
		t.Fatalf("expected 1 built-in log source, got %d", len(fw.Logs))
	}
	if fw.Logs[0].Path != "storage/logs/*.log" {
		t.Errorf("expected built-in log path, got %s", fw.Logs[0].Path)
	}
}

// ── Custom framework with Logs ───────────────────────────────────────────────

func TestGetFrameworkCustom_WithLogs(t *testing.T) {
	setConfigDir(t)

	dir := FrameworksDir()
	os.MkdirAll(dir, 0755)

	fw := Framework{
		Name:      "symfony",
		Label:     "Symfony",
		PublicDir: "public",
		Detect:    []FrameworkRule{{File: "symfony.lock"}},
		Logs: []FrameworkLogSource{
			{Path: "var/log/*.log", Format: "raw"},
		},
	}
	data, _ := yaml.Marshal(fw)
	os.WriteFile(filepath.Join(dir, "symfony.yaml"), data, 0644)

	got, ok := GetFramework("symfony")
	if !ok {
		t.Fatal("expected to find symfony")
	}
	if len(got.Logs) != 1 {
		t.Fatalf("expected 1 log source, got %d", len(got.Logs))
	}
	if got.Logs[0].Path != "var/log/*.log" {
		t.Errorf("log path = %q", got.Logs[0].Path)
	}
	if got.Logs[0].Format != "raw" {
		t.Errorf("log format = %q", got.Logs[0].Format)
	}
}

// The built-in Symfony def must follow Symfony's env convention: lerd writes its
// connection values into .env.local (the gitignored local override), seeded from
// the committed .env. Keeping the built-in aligned with the store def stops the
// two from contradicting each other on the offline fallback path.
func TestGetFrameworkSymfony_BuiltinEnvTargetsEnvLocal(t *testing.T) {
	setConfigDir(t)

	fw, ok := GetFramework("symfony")
	if !ok {
		t.Fatal("GetFramework(symfony): not found")
	}
	if fw.Env.File != ".env.local" {
		t.Errorf("Env.File = %q, want .env.local", fw.Env.File)
	}
	if fw.Env.ExampleFile != ".env" {
		t.Errorf("Env.ExampleFile = %q, want .env", fw.Env.ExampleFile)
	}
	if fw.Env.URLKey != "DEFAULT_URI" {
		t.Errorf("Env.URLKey = %q, want DEFAULT_URI", fw.Env.URLKey)
	}
}

func TestGetFrameworkCustom_WithoutLogs(t *testing.T) {
	setConfigDir(t)

	dir := FrameworksDir()
	os.MkdirAll(dir, 0755)

	fw := Framework{
		Name:      "wordpress",
		Label:     "WordPress",
		PublicDir: ".",
		Detect:    []FrameworkRule{{File: "wp-login.php"}},
	}
	data, _ := yaml.Marshal(fw)
	os.WriteFile(filepath.Join(dir, "wordpress.yaml"), data, 0644)

	got, ok := GetFramework("wordpress")
	if !ok {
		t.Fatal("expected to find wordpress")
	}
	if len(got.Logs) != 0 {
		t.Errorf("expected 0 log sources for wordpress, got %d", len(got.Logs))
	}
}

// ── GetFrameworkForScaffold ──────────────────────────────────────────────────

// installStoreFrameworkAged installs a store definition and back-dates it, so a
// test can place one either inside or outside the store's refresh window.
func installStoreFrameworkAged(t *testing.T, fw *Framework, age time.Duration) {
	t.Helper()
	installStoreFramework(t, fw)
	path := filepath.Join(StoreFrameworksDir(), fw.Name+".yaml")
	if fw.Version != "" {
		path = filepath.Join(StoreFrameworksDir(), fw.Name+"@"+fw.Version+".yaml")
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("Chtimes(%s): %v", path, err)
	}
}

// A framework the store publishes but the machine hasn't installed must be
// fetched on demand, so `lerd new --framework=X` can scaffold a project type
// you've never built before.
func TestGetFrameworkForScaffold_FetchesUninstalled(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })

	called := ""
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		called = name
		fw := &Framework{Name: name, Label: "CodeIgniter", Version: "4", PublicDir: "public"}
		if err := SaveStoreFramework(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}

	got, ok := GetFrameworkForScaffold("codeigniter", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(codeigniter): not found after fetch")
	}
	if got.Label != "CodeIgniter" {
		t.Errorf("Label = %q, want CodeIgniter", got.Label)
	}
	if called != "codeigniter" {
		t.Errorf("fetch hook called with %q, want codeigniter", called)
	}
}

// A built-in name must still go to the store: the create command in the binary
// is a snapshot, and scaffolding on a fresh install has to use the published one.
func TestGetFrameworkForScaffold_PrefersStoreOverBuiltin(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })

	const published = "composer create-project laravel/laravel --published"
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		fw := &Framework{Name: name, Label: "Laravel", Version: "12", PublicDir: "public", Create: published}
		if err := SaveStoreFramework(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel): not found")
	}
	if got.Create != published {
		t.Errorf("Create = %q, want the store's %q", got.Create, published)
	}
}

// A published definition with no create command must not cost a built-in name
// the one it has. Store data reaches every binary within the day and nothing
// gates it per version, so `lerd new laravel` has to keep working through a
// definition that drops the field or renames it.
func TestGetFrameworkForScaffold_BuiltinSurvivesADefinitionThatCannotScaffold(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		fw := &Framework{Name: name, Label: "Laravel", Version: "13", PublicDir: "public"}
		if err := SaveStoreFramework(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel): not found")
	}
	if got.Create != laravelFramework.Create {
		t.Errorf("Create = %q, want the built-in %q", got.Create, laravelFramework.Create)
	}
}

// A framework's newest major can ship no project skeleton at all while the ones
// before it do, and a run that pinned nothing did not ask for that major. It
// scaffolds from the newest one that can rather than refusing the framework.
func TestGetFrameworkForScaffold_FallsBackToTheNewestMajorThatScaffolds(t *testing.T) {
	setConfigDir(t)

	const create = "composer create-project laravel/lumen:^10.0"
	installStoreFramework(t, &Framework{Name: "lumen", Label: "Lumen", Version: "11", PublicDir: "public"})
	installStoreFramework(t, &Framework{Name: "lumen", Label: "Lumen", Version: "10", PublicDir: "public", Create: create})

	got, ok := GetFrameworkForScaffold("lumen", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(lumen): not found")
	}
	if got.Create != create {
		t.Errorf("Create = %q, want %q", got.Create, create)
	}
	if got.Version != "10" {
		t.Errorf("Version = %q, want the newest major that scaffolds, 10", got.Version)
	}
}

// A pinned major is the one asked for. Answering it with an older skeleton would
// lay down a project of a different major than the one requested.
func TestGetFrameworkForScaffold_KeepsAPinnedMajorThatCannotScaffold(t *testing.T) {
	setConfigDir(t)

	installStoreFramework(t, &Framework{Name: "lumen", Label: "Lumen", Version: "11", PublicDir: "public"})
	installStoreFramework(t, &Framework{Name: "lumen", Label: "Lumen", Version: "10", PublicDir: "public",
		Create: "composer create-project laravel/lumen:^10.0"})

	got, ok := GetFrameworkForScaffold("lumen", "11")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(lumen, 11): not found")
	}
	if got.Create != "" {
		t.Errorf("Create = %q, want none: major 11 ships no skeleton", got.Create)
	}
}

// An unreachable store leaves the built-in as the scaffold definition, so
// `lerd new` still works offline.
func TestGetFrameworkForScaffold_FallsBackToBuiltin(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		return nil, os.ErrNotExist
	}

	got, ok := GetFrameworkForScaffold("laravel", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel): built-in not found")
	}
	if got.Create != laravelFramework.Create {
		t.Errorf("Create = %q, want the built-in %q", got.Create, laravelFramework.Create)
	}
}

// A definition installed inside the refresh window is already current, so a
// repeat scaffold must not spend a store round trip on it.
func TestGetFrameworkForScaffold_SkipsFetchWhenFresh(t *testing.T) {
	setConfigDir(t)

	installStoreFrameworkAged(t, &Framework{
		Name: "laravel", Label: "Laravel", Version: "12", PublicDir: "public",
		Create: "composer create-project laravel/laravel --fresh",
	}, time.Minute)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		t.Fatalf("fetch hook called for a fresh definition (%q)", name)
		return nil, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel): not found")
	}
	if got.Create != "composer create-project laravel/laravel --fresh" {
		t.Errorf("Create = %q, want the installed definition's", got.Create)
	}
}

// A stale definition is re-fetched, and when the store cannot be reached the
// stale copy still beats the built-in: it is the newer of the two.
func TestGetFrameworkForScaffold_KeepsStaleWhenStoreUnreachable(t *testing.T) {
	setConfigDir(t)

	installStoreFrameworkAged(t, &Framework{
		Name: "laravel", Label: "Laravel", Version: "12", PublicDir: "public",
		Create: "composer create-project laravel/laravel --stale",
	}, 48*time.Hour)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	fetched := false
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		fetched = true
		return nil, os.ErrNotExist
	}

	got, ok := GetFrameworkForScaffold("laravel", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel): not found")
	}
	if !fetched {
		t.Error("stale definition was not re-fetched")
	}
	if got.Create != "composer create-project laravel/laravel --stale" {
		t.Errorf("Create = %q, want the installed definition's", got.Create)
	}
}

// With several versions installed, the newest one scaffolds. Version numbers are
// compared as numbers: as strings, @9 sorts above @12.
func TestGetFrameworkForScaffold_PrefersHighestInstalledVersion(t *testing.T) {
	setConfigDir(t)

	installStoreFrameworkAged(t, &Framework{
		Name: "laravel", Label: "Laravel", Version: "9", PublicDir: "public",
		Create: "composer create-project laravel/laravel --nine",
	}, time.Minute)
	installStoreFrameworkAged(t, &Framework{
		Name: "laravel", Label: "Laravel", Version: "12", PublicDir: "public",
		Create: "composer create-project laravel/laravel --twelve",
	}, time.Minute)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		t.Fatalf("fetch hook called for a fresh definition (%q)", name)
		return nil, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel): not found")
	}
	if got.Create != "composer create-project laravel/laravel --twelve" {
		t.Errorf("Create = %q, want Laravel 12's", got.Create)
	}
}

// A pinned major scaffolds that major's definition, not the newest one on disk.
// Someone starting a project on an older release picked it deliberately.
func TestGetFrameworkForScaffold_PinnedVersionWins(t *testing.T) {
	setConfigDir(t)

	installStoreFrameworkAged(t, &Framework{
		Name: "laravel", Label: "Laravel", Version: "11", PublicDir: "public",
		Create: "composer create-project laravel/laravel --eleven",
	}, time.Minute)
	installStoreFrameworkAged(t, &Framework{
		Name: "laravel", Label: "Laravel", Version: "13", PublicDir: "public",
		Create: "composer create-project laravel/laravel --thirteen",
	}, time.Minute)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		t.Fatalf("fetch hook called for a fresh pinned definition (%q@%q)", name, version)
		return nil, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "11")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel, 11): not found")
	}
	if got.Create != "composer create-project laravel/laravel --eleven" {
		t.Errorf("Create = %q, want Laravel 11's", got.Create)
	}
}

// A pinned major that is not installed is fetched by that version, so choosing
// an older release in a wizard does not silently scaffold the latest.
func TestGetFrameworkForScaffold_FetchesThePinnedVersion(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	asked := ""
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		asked = version
		fw := &Framework{Name: name, Label: "Laravel", Version: version, PublicDir: "public",
			Create: "composer create-project laravel/laravel --v" + version}
		if err := SaveStoreFramework(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "10")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel, 10): not found")
	}
	if asked != "10" {
		t.Errorf("fetch hook asked for version %q, want 10", asked)
	}
	if got.Create != "composer create-project laravel/laravel --v10" {
		t.Errorf("Create = %q, want the fetched Laravel 10 definition's", got.Create)
	}
}

// A major nothing publishes resolves as if none had been pinned, rather than
// reporting a framework lerd knows perfectly well as unknown.
func TestGetFrameworkForScaffold_UnservedPinFallsBackToLatest(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		if version != "" {
			return nil, os.ErrNotExist
		}
		fw := &Framework{Name: name, Label: "Laravel", Version: "13", PublicDir: "public",
			Create: "composer create-project laravel/laravel --latest"}
		if err := SaveStoreFramework(fw); err != nil {
			return nil, err
		}
		return fw, nil
	}

	got, ok := GetFrameworkForScaffold("laravel", "99")
	if !ok {
		t.Fatal("GetFrameworkForScaffold(laravel, 99): not found")
	}
	if got.Create != "composer create-project laravel/laravel --latest" {
		t.Errorf("Create = %q, want the latest definition's", got.Create)
	}
}

// A name the store doesn't publish keeps the original not-found result.
func TestGetFrameworkForScaffold_UnknownStaysNotFound(t *testing.T) {
	setConfigDir(t)

	prev := frameworkFetchHook
	t.Cleanup(func() { frameworkFetchHook = prev })
	frameworkFetchHook = func(name, version string) (*Framework, error) {
		return nil, os.ErrNotExist
	}

	if _, ok := GetFrameworkForScaffold("nonexistent", ""); ok {
		t.Error("GetFrameworkForScaffold(nonexistent) = true, want false")
	}
}

// ── SaveFramework preserves Logs ─────────────────────────────────────────────

func TestSaveFrameworkLaravel_PersistsLogs(t *testing.T) {
	setConfigDir(t)

	fw := &Framework{
		Name: "laravel",
		Workers: map[string]FrameworkWorker{
			"horizon": {Label: "Horizon", Command: "php artisan horizon"},
		},
		Logs: []FrameworkLogSource{
			{Path: "storage/logs/*.log", Format: "monolog"},
			{Path: "storage/logs/jobs/*.log", Format: "monolog"},
		},
	}
	if err := SaveFramework(fw); err != nil {
		t.Fatal(err)
	}

	// Read back raw YAML to verify logs are persisted
	path := filepath.Join(FrameworksDir(), "laravel.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved Framework
	if err := yaml.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Logs) != 2 {
		t.Fatalf("expected 2 log sources saved, got %d", len(saved.Logs))
	}
}

func TestSaveFrameworkCustom_PersistsLogs(t *testing.T) {
	setConfigDir(t)

	fw := &Framework{
		Name:      "drupal",
		Label:     "Drupal",
		PublicDir: "web",
		Logs: []FrameworkLogSource{
			{Path: "sites/default/files/logs/*.log"},
		},
	}
	if err := SaveFramework(fw); err != nil {
		t.Fatal(err)
	}

	got, ok := GetFramework("drupal")
	if !ok {
		t.Fatal("expected to find drupal after save")
	}
	if len(got.Logs) != 1 {
		t.Fatalf("expected 1 log source, got %d", len(got.Logs))
	}
}

// ── ListFrameworks includes Logs ─────────────────────────────────────────────

func TestListFrameworks_IncludesLogs(t *testing.T) {
	setConfigDir(t)

	frameworks := ListFrameworks()
	// At minimum the built-in Laravel
	found := false
	for _, fw := range frameworks {
		if fw.Name == "laravel" {
			found = true
			if len(fw.Logs) == 0 {
				t.Error("ListFrameworks: laravel should have Logs")
			}
		}
	}
	if !found {
		t.Error("ListFrameworks should include laravel")
	}
}

// ── RemoveFramework ─────────────────────────────────────────────────────────

func TestRemoveFramework_UserDefined(t *testing.T) {
	setConfigDir(t)

	dir := FrameworksDir()
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "myfw.yaml"), []byte("name: myfw\n"), 0644)

	if err := RemoveFramework("myfw"); err != nil {
		t.Fatalf("RemoveFramework(user): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "myfw.yaml")); !os.IsNotExist(err) {
		t.Error("expected user file to be removed")
	}
}

func TestRemoveFramework_StoreInstalled(t *testing.T) {
	setConfigDir(t)

	dir := StoreFrameworksDir()
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "symfony.yaml"), []byte("name: symfony\n"), 0644)
	os.WriteFile(filepath.Join(dir, "symfony@7.yaml"), []byte("name: symfony\nversion: \"7\"\n"), 0644)

	if err := RemoveFramework("symfony"); err != nil {
		t.Fatalf("RemoveFramework(store): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "symfony.yaml")); !os.IsNotExist(err) {
		t.Error("expected unversioned store file to be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "symfony@7.yaml")); !os.IsNotExist(err) {
		t.Error("expected versioned store file to be removed")
	}
}

func TestRemoveFramework_NotFound(t *testing.T) {
	setConfigDir(t)

	err := RemoveFramework("nonexistent")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

// ── FrameworkLogSource YAML round-trip ────────────────────────────────────────

func TestFrameworkLogSource_YAMLRoundTrip(t *testing.T) {
	original := []FrameworkLogSource{
		{Path: "storage/logs/*.log", Format: "monolog"},
		{Path: "var/log/*.log", Format: "raw"},
		{Path: "logs/*.txt"}, // no format
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var loaded []FrameworkLogSource
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3, got %d", len(loaded))
	}
	if loaded[0].Path != "storage/logs/*.log" || loaded[0].Format != "monolog" {
		t.Errorf("entry 0: %+v", loaded[0])
	}
	if loaded[1].Format != "raw" {
		t.Errorf("entry 1 format: %q", loaded[1].Format)
	}
	if loaded[2].Format != "" {
		t.Errorf("entry 2 format should be empty, got %q", loaded[2].Format)
	}
}

// ValidatePublicDir guards the nginx document root from a hostile .lerd.yaml
// whose public_dir points outside the project, e.g. ../../etc.
func TestValidatePublicDir(t *testing.T) {
	good := []string{"", ".", "public", "web", "public_html", "src/public"}
	for _, s := range good {
		if err := ValidatePublicDir(s); err != nil {
			t.Errorf("ValidatePublicDir(%q) = %v, want nil", s, err)
		}
	}
	bad := []string{
		"..",
		"../etc",
		"../../etc",
		"public/../etc",
		"public/..",
		"/etc",
		"/etc/passwd",
		"~/.ssh",
		"public\x00evil",
	}
	for _, s := range bad {
		if err := ValidatePublicDir(s); err == nil {
			t.Errorf("ValidatePublicDir(%q) = nil, want error", s)
		}
	}
}

// ── SitesUsingFramework / InstalledFrameworkNames ────────────────────────────

func TestSitesUsingFramework(t *testing.T) {
	setConfigDir(t)

	reg := &SiteRegistry{Sites: []Site{
		{Name: "shop", Framework: "laravel"},
		{Name: "blog", Framework: "wordpress"},
		{Name: "api", Framework: "laravel"},
		{Name: "static", Framework: ""},
	}}
	if err := SaveSites(reg); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}

	got := SitesUsingFramework("laravel")
	if len(got) != 2 || got[0] != "shop" || got[1] != "api" {
		t.Errorf("SitesUsingFramework(laravel) = %v, want [shop api]", got)
	}
	if got := SitesUsingFramework("symfony"); len(got) != 0 {
		t.Errorf("SitesUsingFramework(symfony) = %v, want empty", got)
	}
}

func TestSitesUsingFramework_IgnoresUnlinkedParkedEntries(t *testing.T) {
	setConfigDir(t)

	reg := &SiteRegistry{Sites: []Site{
		{Name: "served", Framework: "wordpress"},
		{Name: "parked", Framework: "wordpress", Ignored: true},
	}}
	if err := SaveSites(reg); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}

	got := SitesUsingFramework("wordpress")
	if len(got) != 1 || got[0] != "served" {
		t.Errorf("SitesUsingFramework(wordpress) = %v, want [served]", got)
	}
}

func TestInstalledFrameworkNames_ExcludesBuiltins(t *testing.T) {
	setConfigDir(t)

	user := FrameworksDir()
	os.MkdirAll(user, 0755)
	os.WriteFile(filepath.Join(user, "custom.yaml"), []byte("name: custom\n"), 0644)
	os.WriteFile(filepath.Join(user, "laravel.yaml"), []byte("name: laravel\n"), 0644)

	store := StoreFrameworksDir()
	os.MkdirAll(store, 0755)
	os.WriteFile(filepath.Join(store, "symfony.yaml"), []byte("name: symfony\n"), 0644)
	os.WriteFile(filepath.Join(store, "symfony@7.yaml"), []byte("name: symfony\nversion: \"7\"\n"), 0644)
	os.WriteFile(filepath.Join(store, "wordpress@6.yaml"), []byte("name: wordpress\n"), 0644)

	got := InstalledFrameworkNames()
	want := []string{"custom", "wordpress"}
	if len(got) != len(want) {
		t.Fatalf("InstalledFrameworkNames() = %v, want %v (built-ins laravel/symfony must be excluded)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("InstalledFrameworkNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUnusedInstalledFrameworks(t *testing.T) {
	setConfigDir(t)

	store := StoreFrameworksDir()
	os.MkdirAll(store, 0755)
	os.WriteFile(filepath.Join(store, "wordpress.yaml"), []byte("name: wordpress\n"), 0644)
	os.WriteFile(filepath.Join(store, "drupal.yaml"), []byte("name: drupal\n"), 0644)

	reg := &SiteRegistry{Sites: []Site{
		{Name: "blog", Framework: "wordpress"},
		{Name: "old", Framework: "drupal", Ignored: true},
	}}
	if err := SaveSites(reg); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}

	got := UnusedInstalledFrameworks()
	if len(got) != 1 || got[0] != "drupal" {
		t.Errorf("UnusedInstalledFrameworks() = %v, want [drupal]", got)
	}
}

func TestListFrameworkFiles_IncludesUserVersioned(t *testing.T) {
	setConfigDir(t)

	user := FrameworksDir()
	os.MkdirAll(user, 0755)
	versioned := filepath.Join(user, "drupal@10.yaml")
	os.WriteFile(versioned, []byte("name: drupal\nversion: \"10\"\n"), 0644)

	files := ListFrameworkFiles("drupal")
	if len(files) != 1 || files[0].Path != versioned || files[0].Version != "10" {
		t.Fatalf("ListFrameworkFiles(drupal) = %+v, want the user-dir drupal@10 file", files)
	}

	if err := RemoveFramework("drupal"); err != nil {
		t.Fatalf("RemoveFramework: %v", err)
	}
	if _, err := os.Stat(versioned); !os.IsNotExist(err) {
		t.Error("expected user-dir versioned file to be removed")
	}
}
