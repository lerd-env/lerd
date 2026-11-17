package plugins

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Cond is a parsed site tab `when`: an operator over Args, or one condition.
// The dashboard gets it as JSON and evaluates it against the checkout it shows.
type Cond struct {
	Op   string  `json:"op,omitempty"` // and, or, not
	Args []*Cond `json:"args,omitempty"`
	// Is names the condition: database or framework.
	Is string `json:"is,omitempty"`
	// Name is the framework pattern, with * as a wildcard.
	Name string `json:"name,omitempty"`
	// Cmp and Version compare the framework's major version when Version is set.
	Cmp     string `json:"cmp,omitempty"`
	Version *int   `json:"version,omitempty"`
}

// Checkout is what a condition reads about one checkout of a site, named as
// the dashboard's site list names it.
type Checkout struct {
	HasDatabase      bool   `json:"has_database,omitempty"`
	Framework        string `json:"framework,omitempty"`
	FrameworkVersion string `json:"framework_version,omitempty"`
}

const (
	maxWhenLen   = 256
	maxWhenDepth = 8
)

var (
	whenTokenRe = regexp.MustCompile(`\(|\)|[^\s()]+`)
	condNameRe  = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	patternRe   = regexp.MustCompile(`^[a-z0-9*][a-z0-9._*-]*$`)
	versionRe   = regexp.MustCompile(`^(>=|<=|>|<)?([0-9]+)$`)
)

// ParseWhen parses a site tab's when. An empty one is nil, which always applies.
func ParseWhen(s string) (*Cond, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	if len(s) > maxWhenLen {
		return nil, fmt.Errorf("longer than %d characters", maxWhenLen)
	}
	p := &whenParser{tokens: whenTokenRe.FindAllString(s, -1)}
	c, err := p.or(0)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected %q", p.tokens[p.pos])
	}
	return c, nil
}

type whenParser struct {
	tokens []string
	pos    int
}

func (p *whenParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *whenParser) or(depth int) (*Cond, error) {
	return p.chain("or", depth, p.and)
}

func (p *whenParser) and(depth int) (*Cond, error) {
	return p.chain("and", depth, p.unary)
}

// chain reads operands joined by op into one flat node.
func (p *whenParser) chain(op string, depth int, next func(int) (*Cond, error)) (*Cond, error) {
	first, err := next(depth)
	if err != nil {
		return nil, err
	}
	args := []*Cond{first}
	for p.peek() == op {
		p.pos++
		c, err := next(depth)
		if err != nil {
			return nil, err
		}
		args = append(args, c)
	}
	if len(args) == 1 {
		return first, nil
	}
	return &Cond{Op: op, Args: args}, nil
}

func (p *whenParser) unary(depth int) (*Cond, error) {
	if p.peek() == "not" {
		p.pos++
		c, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		return &Cond{Op: "not", Args: []*Cond{c}}, nil
	}
	return p.primary(depth)
}

func (p *whenParser) primary(depth int) (*Cond, error) {
	tok := p.peek()
	switch tok {
	case "":
		if p.pos == 0 {
			return nil, fmt.Errorf("expected a condition")
		}
		return nil, fmt.Errorf("expected a condition after %q", p.tokens[p.pos-1])
	case "(":
		if depth >= maxWhenDepth {
			return nil, fmt.Errorf("nested deeper than %d", maxWhenDepth)
		}
		p.pos++
		c, err := p.or(depth + 1)
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("unbalanced parenthesis")
		}
		p.pos++
		return c, nil
	case ")", "and", "or":
		return nil, fmt.Errorf("expected a condition, got %q", tok)
	}
	p.pos++
	return parseCondition(tok)
}

func parseCondition(tok string) (*Cond, error) {
	name, value, hasValue := strings.Cut(tok, ":")
	if !condNameRe.MatchString(name) {
		return nil, fmt.Errorf("%q is not a condition; conditions are lowercase", tok)
	}
	switch name {
	case "database":
		if hasValue {
			return nil, fmt.Errorf("database takes no value, got %q", tok)
		}
		return &Cond{Is: name}, nil
	case "framework":
		return parseFramework(value)
	}
	return nil, fmt.Errorf("%s is not a condition this lerd knows; use database or framework:<name>", name)
}

func parseFramework(value string) (*Cond, error) {
	if value == "" {
		return nil, fmt.Errorf("framework needs a name, like framework:laravel")
	}
	pattern, version, hasVersion := strings.Cut(value, "@")
	if !patternRe.MatchString(pattern) {
		return nil, fmt.Errorf("framework name %q may hold only lowercase letters, digits, ., _, - and *", pattern)
	}
	c := &Cond{Is: "framework", Name: pattern}
	if !hasVersion {
		return c, nil
	}
	m := versionRe.FindStringSubmatch(version)
	if m == nil {
		return nil, fmt.Errorf("framework version %q is not one this lerd knows; use a major like @12, or @>=, @<=, @> or @< before it", version)
	}
	n, _ := strconv.Atoi(m[2])
	c.Cmp, c.Version = m[1], &n
	return c, nil
}

// Applies reports whether the condition holds for a checkout; nil always does.
func (c *Cond) Applies(co Checkout) bool {
	if c == nil {
		return true
	}
	switch c.Op {
	case "and":
		for _, a := range c.Args {
			if !a.Applies(co) {
				return false
			}
		}
		return true
	case "or":
		for _, a := range c.Args {
			if a.Applies(co) {
				return true
			}
		}
		return false
	case "not":
		return !c.Args[0].Applies(co)
	}
	switch c.Is {
	case "database":
		return co.HasDatabase
	case "framework":
		return c.matchesFramework(co)
	}
	return false
}

func (c *Cond) matchesFramework(co Checkout) bool {
	name := strings.ToLower(co.Framework)
	if name == "" {
		return false
	}
	if ok, _ := path.Match(c.Name, name); !ok {
		return false
	}
	if c.Version == nil {
		return true
	}
	have, err := strconv.Atoi(co.FrameworkVersion)
	if err != nil {
		return false
	}
	want := *c.Version
	switch c.Cmp {
	case ">=":
		return have >= want
	case "<=":
		return have <= want
	case ">":
		return have > want
	case "<":
		return have < want
	}
	return have == want
}
