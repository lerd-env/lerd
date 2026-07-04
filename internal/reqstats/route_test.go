package reqstats

import "testing"

func TestNormalizeRoute(t *testing.T) {
	cases := []struct {
		method, uri string
		want        string
	}{
		{"GET", "/users/123", "GET /users/:id"},
		{"get", "/users/123/posts/456", "GET /users/:id/posts/:id"},
		{"GET", "/reports/5?page=2&sort=asc", "GET /reports/:id"},
		{"POST", "/import", "POST /import"},
		{"GET", "/", "GET /"},
		{"GET", "", "GET /"},
		{"GET", "/users/550e8400-e29b-41d4-a716-446655440000", "GET /users/:id"},
		{"GET", "/assets/a1b2c3d4e5f6a7b8/app.css", "GET /assets/:id/app.css"},
		{"GET", "/v1/orders/42", "GET /v1/orders/:id"},
		{"GET", "/users/123/", "GET /users/:id"},
		{"GET", "/search#frag", "GET /search"},
	}
	for _, c := range cases {
		if got := NormalizeRoute(c.method, c.uri); got != c.want {
			t.Errorf("NormalizeRoute(%q, %q) = %q, want %q", c.method, c.uri, got, c.want)
		}
	}
}

func TestNormalizeRouteKeepsWords(t *testing.T) {
	// Version tags and normal path words must not collapse to :id.
	if got := NormalizeRoute("GET", "/api/v2/dashboard"); got != "GET /api/v2/dashboard" {
		t.Errorf("kept-words route = %q", got)
	}
}
