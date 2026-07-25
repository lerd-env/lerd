package ui

import "testing"

// A mongo-style site wires a single DSN instead of a DB_HOST/DB_DATABASE pair,
// so the owning site is read from the URL path of whichever env value points
// at this engine.
func TestDsnDatabase(t *testing.T) {
	cases := []struct {
		value, service, want string
	}{
		{"mongodb://root:lerd@lerd-mongo:27017/astrolov?authSource=admin", "mongo", "astrolov"},
		{"mongodb://root:lerd@lerd-mongo:27017/?authSource=admin", "mongo", ""},
		{"mysql://root:lerd@lerd-mysql:3306/shop", "mysql", "shop"},
		// Another service's DSN must not claim this engine's databases.
		{"mongodb://root:lerd@lerd-mongo:27017/astrolov", "mysql", ""},
		// An alternate instance is not the canonical one.
		{"mongodb://root:lerd@lerd-mongo-6:27017/astrolov", "mongo", ""},
		{"not a url at all", "mongo", ""},
		{"redis://lerd-redis:6379", "redis", ""},
		// A multi-segment path is not a database name.
		{"http://lerd-rustfs:9000/bucket/key", "rustfs", ""},
	}
	for _, tc := range cases {
		if got := dsnDatabase(tc.value, tc.service); got != tc.want {
			t.Errorf("dsnDatabase(%q, %q) = %q, want %q", tc.value, tc.service, got, tc.want)
		}
	}
}

// The declared actions arrive as a map; the row renders them in a stable
// order with anything destructive at the end.
func TestSortEntityActions(t *testing.T) {
	actions := []entityActionResponse{
		{Name: "delete", Destructive: true},
		{Name: "flush"},
		{Name: "import"},
		{Name: "create"},
		{Name: "export"},
	}
	sortEntityActions(actions)
	want := []string{"create", "export", "import", "flush", "delete"}
	for i, w := range want {
		if actions[i].Name != w {
			t.Fatalf("order = %v, want %v", actions, want)
		}
	}
}
