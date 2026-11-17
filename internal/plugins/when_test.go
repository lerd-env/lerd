package plugins

import (
	"strings"
	"testing"
)

func TestParseWhen_Applies(t *testing.T) {
	laravel12 := Checkout{HasDatabase: true, Framework: "laravel", FrameworkVersion: "12"}
	laravel10 := Checkout{Framework: "laravel", FrameworkVersion: "10"}
	legacy := Checkout{Framework: "laravel-legacy"}
	wordpress := Checkout{HasDatabase: true, Framework: "wordpress", FrameworkVersion: "6"}
	plainDB := Checkout{HasDatabase: true}
	static := Checkout{}

	cases := []struct {
		when string
		yes  []Checkout
		no   []Checkout
	}{
		{"", []Checkout{static, laravel12}, nil},
		{"database", []Checkout{laravel12, plainDB}, []Checkout{static, laravel10}},
		{"framework:laravel", []Checkout{laravel12, laravel10}, []Checkout{legacy, wordpress, static}},
		{"framework:laravel*", []Checkout{laravel12, legacy}, []Checkout{wordpress}},
		{"framework:*", []Checkout{laravel12, legacy, wordpress}, []Checkout{plainDB, static}},
		{"not framework:*", []Checkout{plainDB, static}, []Checkout{laravel12}},
		{"framework:LARAVEL-x", nil, nil},
		{"framework:laravel@12", []Checkout{laravel12}, []Checkout{laravel10, legacy}},
		{"framework:laravel@>=11", []Checkout{laravel12}, []Checkout{laravel10}},
		{"framework:laravel@<=10", []Checkout{laravel10}, []Checkout{laravel12}},
		{"framework:laravel@>10", []Checkout{laravel12}, []Checkout{laravel10}},
		{"framework:laravel@<12", []Checkout{laravel10}, []Checkout{laravel12}},
		{"framework:laravel@>=10 and framework:laravel@<12", []Checkout{laravel10}, []Checkout{laravel12}},
		{"framework:*@>=10", []Checkout{laravel12, laravel10}, []Checkout{wordpress, legacy}},
		{"framework:acme-cms", nil, []Checkout{laravel12, static}},
		// and binds tighter than or, so the parentheses change nothing here.
		{"database and not framework:wordpress or framework:laravel*@>=11", []Checkout{laravel12, plainDB}, []Checkout{wordpress, laravel10, static}},
		{"(database and not framework:wordpress) or framework:laravel*@>=11", []Checkout{laravel12, plainDB}, []Checkout{wordpress, laravel10, static}},
		{"database and (framework:wordpress or framework:laravel)", []Checkout{laravel12, wordpress}, []Checkout{plainDB, laravel10}},
		{"not not database", []Checkout{plainDB}, []Checkout{static}},
	}
	for _, c := range cases {
		cond, err := ParseWhen(c.when)
		if c.when == "framework:LARAVEL-x" {
			if err == nil {
				t.Errorf("%q should be refused", c.when)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseWhen(%q): %v", c.when, err)
			continue
		}
		for _, co := range c.yes {
			if !cond.Applies(co) {
				t.Errorf("%q should apply to %+v", c.when, co)
			}
		}
		for _, co := range c.no {
			if cond.Applies(co) {
				t.Errorf("%q should not apply to %+v", c.when, co)
			}
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
