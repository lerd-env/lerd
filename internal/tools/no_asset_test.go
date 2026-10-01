package tools

import (
	"errors"
	"strings"
	"testing"
)

func TestURLReportsAMissingPlatformAsNoAsset(t *testing.T) {
	m := &Manifest{Tools: map[string]Tool{
		"mise": {Version: "1.0.0", URL: "https://x/{asset}", Assets: map[string]string{"linux/amd64": "mise-{version}.tgz"}},
	}}
	if _, err := m.URL("mise", "linux", "amd64"); err != nil {
		t.Fatalf("a published platform must resolve: %v", err)
	}
	_, err := m.URL("mise", "windows", "amd64")
	if !errors.Is(err, ErrNoAsset) {
		t.Fatalf("errors.Is(err, ErrNoAsset) = false for %v", err)
	}
	if !strings.Contains(err.Error(), "mise has no release asset for windows/amd64") {
		t.Errorf("the message callers and users already see changed: %q", err)
	}
	if _, err := m.URL("nope", "linux", "amd64"); errors.Is(err, ErrNoAsset) {
		t.Error("an unknown tool is a different failure from a missing platform")
	}
}

func TestMkcertIsPublishedForWindows(t *testing.T) {
	m := embeddedManifest()
	var err error
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		url, err := m.URL("mkcert", "windows", arch)
		if err != nil {
			t.Fatalf("mkcert has no windows/%s asset: %v", arch, err)
		}
		if !strings.HasSuffix(url, "-windows-"+arch+".exe") {
			t.Errorf("windows/%s url = %s", arch, url)
		}
	}
}
