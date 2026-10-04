package ui

import "testing"

func TestInlineBindings(t *testing.T) {
	cases := []struct {
		sql      string
		bindings []interface{}
		want     string
	}{
		{"select * from users where id = ?", []interface{}{float64(7)}, "select * from users where id = 7"},
		{"insert into t (a, b, c, d) values (?, ?, ?, ?)", []interface{}{"O'Hara", nil, true, 1.5}, "insert into t (a, b, c, d) values ('O''Hara', NULL, 1, 1.5)"},
		{"select '?' as q, ? as v", []interface{}{"x"}, "select '?' as q, 'x' as v"},
		{"select ? , ?", []interface{}{float64(1)}, "select 1 , ?"},
		{"select 1", nil, "select 1"},
	}
	for _, c := range cases {
		if got := inlineBindings(c.sql, c.bindings); got != c.want {
			t.Errorf("inlineBindings(%q) = %q, want %q", c.sql, got, c.want)
		}
	}
}
