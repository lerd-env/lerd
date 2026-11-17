package plugins

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var updateWhenCases = flag.Bool("update", false, "rewrite testdata/when_cases.json with the parsed conditions")

// whenCase is one row of testdata/when_cases.json, which the dashboard's tests
// read too, so lerd and the dashboard judge every condition the same way.
type whenCase struct {
	When    string          `json:"when"`
	Cond    json.RawMessage `json:"cond"`
	Applies []Checkout      `json:"applies"`
	Skips   []Checkout      `json:"skips"`
}

// unescapedJSON keeps < and > readable in the fixture.
func unescapedJSON(t *testing.T, v any) []byte {
	t.Helper()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

func TestParseWhen_SharedCases(t *testing.T) {
	const file = "testdata/when_cases.json"
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var cases []whenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		cond, err := ParseWhen(c.When)
		if err != nil {
			t.Errorf("ParseWhen(%q): %v", c.When, err)
			continue
		}
		parsed := unescapedJSON(t, cond)
		var want bytes.Buffer
		if *updateWhenCases {
			cases[i].Cond = parsed
		} else if json.Compact(&want, c.Cond) != nil || want.String() != string(parsed) {
			t.Errorf("%q parses to %s, the fixture says %s; rerun with -update", c.When, parsed, c.Cond)
		}
		for _, co := range c.Applies {
			if !cond.Applies(co) {
				t.Errorf("%q should apply to %+v", c.When, co)
			}
		}
		for _, co := range c.Skips {
			if cond.Applies(co) {
				t.Errorf("%q should not apply to %+v", c.When, co)
			}
		}
	}
	if *updateWhenCases {
		var out bytes.Buffer
		enc := json.NewEncoder(&out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cases); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseWhen_Refuses(t *testing.T) {
	cases := map[string]string{
		"database and":                  `after "and"`,
		"(database or framework:yii":    "unbalanced parenthesis",
		"database)":                     `unexpected ")"`,
		"database redis":                `unexpected "redis"`,
		"or database":                   `got "or"`,
		"not":                           `after "not"`,
		"database and redis":            "redis is not a condition this lerd knows",
		"framework":                     "framework needs a name",
		"framework:":                    "framework needs a name",
		"database:mysql":                "database takes no value",
		"Framework:laravel":             "lowercase",
		"framework:Laravel":             "framework name",
		"framework:laravel@":            "version",
		"framework:laravel@=12":         "version",
		"framework:laravel@^12":         "version",
		"framework:laravel@11-12":       "version",
		"framework:laravel@12.1":        "version",
		"framework:laravel@>=":          "version",
		strings.Repeat("database ", 40): "longer than",
		strings.Repeat("(", 9) + "database" + strings.Repeat(")", 9): "nested deeper",
	}
	for when, want := range cases {
		_, err := ParseWhen(when)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseWhen(%q) = %v, want an error containing %q", when, err, want)
		}
	}
}

func TestParseWhen_NestingWithinLimitParses(t *testing.T) {
	when := strings.Repeat("(", 8) + "database" + strings.Repeat(")", 8)
	if _, err := ParseWhen(when); err != nil {
		t.Errorf("eight levels should parse: %v", err)
	}
}
