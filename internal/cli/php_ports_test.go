package cli

import "testing"

func TestParsePortArg(t *testing.T) {
	cases := []struct {
		in         string
		host, cont int
		wantErr    bool
	}{
		{"5173:5173", 5173, 5173, false},
		{"3000", 3000, 3000, false}, // bare number publishes straight through
		{" 8080 : 80 ", 8080, 80, false},
		{"", 0, 0, true},
		{"abc", 0, 0, true},
		{"80:xyz", 0, 0, true},
	}
	for _, c := range cases {
		host, cont, err := parsePortArg(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePortArg(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePortArg(%q): %v", c.in, err)
			continue
		}
		if host != c.host || cont != c.cont {
			t.Errorf("parsePortArg(%q) = %d,%d; want %d,%d", c.in, host, cont, c.host, c.cont)
		}
	}
}
