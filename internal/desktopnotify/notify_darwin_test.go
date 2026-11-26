package desktopnotify

import "testing"

func TestAppInstalled_FollowsTheLerdSchemeHandler(t *testing.T) {
	prev := schemeHandler
	t.Cleanup(func() { schemeHandler = prev })

	schemeHandler = func(string) string { return "" }
	if AppInstalled() {
		t.Error("installed with nothing handling lerd://")
	}
	var asked string
	schemeHandler = func(scheme string) string { asked = scheme; return "/Applications/Lerd.app" }
	if !AppInstalled() {
		t.Error("not installed while an app handles lerd://")
	}
	if asked != "lerd" {
		t.Errorf("asked about scheme %q, want lerd", asked)
	}
}

func TestOpenApp_OpensTheRouteThroughTheScheme(t *testing.T) {
	prev := openURL
	t.Cleanup(func() { openURL = prev })

	var opened string
	openURL = func(u string) error { opened = u; return nil }
	if err := OpenApp("#sites"); err != nil {
		t.Fatal(err)
	}
	if opened != "lerd://open/#sites" {
		t.Errorf("opened %q, want lerd://open/#sites", opened)
	}
}
