package siteops

import "testing"

func TestProjectSlug(t *testing.T) {
	cases := map[string]string{
		"myapp":          "myapp",
		"My App":         "my-app",
		"  my   app  ":   "my-app",
		"Shop_API v2":    "shop-api-v2",
		"acme.com":       "acme.com",
		"--weird--name!": "weird-name",
		"café":           "caf",
	}
	for in, want := range cases {
		if got := ProjectSlug(in); got != want {
			t.Errorf("ProjectSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckProjectName(t *testing.T) {
	for _, good := range []string{"myapp", "my-app", "acme.com", "app2"} {
		if err := CheckProjectName(good); err != nil {
			t.Errorf("CheckProjectName(%q) = %v, want nil", good, err)
		}
	}
	for _, bad := range []string{"My App", "my app", "MyApp", "app!", ""} {
		if err := CheckProjectName(bad); err == nil {
			t.Errorf("CheckProjectName(%q) = nil, want an error", bad)
		}
	}
}

// A folder that already has spaces in its name still links, as a valid domain.
func TestSiteNameAndDomainReplacesSpaces(t *testing.T) {
	name, domain := SiteNameAndDomain("My Shop", "test")
	if name != "my-shop" || domain != "my-shop.test" {
		t.Errorf("got %q, %q; want my-shop, my-shop.test", name, domain)
	}
}
