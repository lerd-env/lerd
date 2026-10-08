package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBrowserLogsResolve_Defaults(t *testing.T) {
	var b *BrowserLogs
	got := b.Resolve()
	want := BrowserLogsSettings{Console: []string{"error", "warn"}, Network: []string{}, Navigation: true, Events: []BrowserLogsEvent{}, Presets: map[string]bool{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestBrowserLogsResolve_ExplicitEmptyListsAndUnknownValues(t *testing.T) {
	off := false
	got := (&BrowserLogs{Enabled: &off, Console: []string{}, Network: []string{"5xx", "3xx", "5xx"}}).Resolve()
	want := BrowserLogsSettings{Enabled: false, Console: []string{}, Network: []string{"5xx"}, Navigation: true, Events: []BrowserLogsEvent{}, Presets: map[string]bool{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestBrowserLogsSettingsValidate_NamesUnknownValue(t *testing.T) {
	err := BrowserLogsSettings{Console: []string{"info"}}.Validate()
	if err == nil || !strings.Contains(err.Error(), `"info"`) {
		t.Fatalf("Validate() = %v, want an error naming \"info\"", err)
	}
}

// Settings land in lerd's registry and never in the project, even one with a
// .lerd.yaml, so opting in leaves the repo untouched.
func TestSaveBrowserLogs_RegistryNeverTheProject(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := t.TempDir()
	lerdYAML := filepath.Join(dir, ".lerd.yaml")
	if err := os.WriteFile(lerdYAML, []byte("php_version: \"8.4\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatal(err)
	}
	s, _ := FindSite("shop")
	want := BrowserLogsSettings{Enabled: true, Console: []string{"error", "warn"}, Network: []string{"4xx"}, Navigation: false, Events: []BrowserLogsEvent{{Event: "inertia:invalid", Label: "Inertia", Message: "detail.response.status"}}, Presets: map[string]bool{"inertia": false}}
	if err := SaveBrowserLogs(*s, want); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(lerdYAML); string(data) != "php_version: \"8.4\"\n" {
		t.Fatalf(".lerd.yaml was touched:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, LocalOverrideFile)); !os.IsNotExist(err) {
		t.Fatalf("%s was created (stat err %v)", LocalOverrideFile, err)
	}
	invalidateSitesCache()
	s, _ = FindSite("shop")
	if got := BrowserLogsFor(*s); !reflect.DeepEqual(got, want) {
		t.Fatalf("BrowserLogsFor = %+v, want %+v", got, want)
	}
}

func TestBrowserLogsEvents_OnlyNamesAndPaths(t *testing.T) {
	good := BrowserLogsEvent{Event: "htmx:responseError", Message: "detail.xhr.status"}
	if err := (BrowserLogsSettings{Events: []BrowserLogsEvent{good}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []BrowserLogsEvent{
		{Event: "x</script><script>alert(1)"},
		{Event: "ok", Message: "detail.a()"},
		{Event: "ok", Message: "detail[0]"},
		{Event: ""},
	} {
		if err := (BrowserLogsSettings{Events: []BrowserLogsEvent{bad}}).Validate(); err == nil {
			t.Errorf("event %+v accepted", bad)
		}
		if got := (&BrowserLogs{Events: []BrowserLogsEvent{bad, good}}).Resolve().Events; len(got) != 1 || got[0] != good {
			t.Errorf("event %+v resolved to %+v", bad, got)
		}
	}
}
