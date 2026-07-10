package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// This file handles the "php-array" env format: a PHP file that returns a
// nested array, as Magento's app/etc/env.php does. Keys are addressed by a
// dotted path (db.connection.default.host). The file is reparsed and reprinted
// rather than patched textually, which is what Magento's own
// DeploymentConfig\Writer does, so comments are not preserved by either.

type phpKind int

const (
	phpString phpKind = iota
	phpInt
	phpBool
	phpNull
	phpFloat
	phpArray
)

// phpValue is a parsed PHP value. Arrays keep their entry order so a rewrite
// does not reshuffle the file.
type phpValue struct {
	kind    phpKind
	str     string
	entries []phpEntry
}

type phpEntry struct {
	key   string
	isInt bool // numeric (list) key, printed unquoted
	val   *phpValue
}

// ReadPhpArray parses a PHP file returning a nested array and flattens it to
// dotted keys. A file with no return statement yields an empty map, not an error.
func ReadPhpArray(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root, err := parsePhpReturn(string(data))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if root == nil {
		return out, nil
	}
	flatten("", root, out)
	return out, nil
}

// ApplyPhpArrayUpdates sets each dotted key to its value, creating intermediate
// arrays as needed, and rewrites the file. A missing file (and its parent dirs)
// is created. An existing scalar keeps its type when the new value fits it.
func ApplyPhpArrayUpdates(path string, updates map[string]string) error {
	var root *phpValue
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if root, err = parsePhpReturn(string(data)); err != nil {
			return err
		}
	case !os.IsNotExist(err):
		return err
	}
	if root == nil || root.kind != phpArray {
		root = &phpValue{kind: phpArray}
	}

	// Sort so the emitted file is deterministic when several new paths are added.
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		setPath(root, strings.Split(k, "."), updates[k])
	}

	var b strings.Builder
	b.WriteString("<?php\nreturn ")
	printValue(&b, root, 0)
	b.WriteString(";\n")

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func flatten(prefix string, v *phpValue, out map[string]string) {
	if v.kind != phpArray {
		out[prefix] = scalarString(v)
		return
	}
	for _, e := range v.entries {
		key := e.key
		if prefix != "" {
			key = prefix + "." + e.key
		}
		flatten(key, e.val, out)
	}
}

func scalarString(v *phpValue) string {
	if v.kind == phpNull {
		return ""
	}
	return v.str
}

// setPath walks (creating) the nested arrays named by segs and assigns value to
// the leaf, preserving the leaf's existing scalar type where the value fits.
func setPath(root *phpValue, segs []string, value string) {
	cur := root
	for i, seg := range segs {
		last := i == len(segs)-1
		idx := -1
		for j := range cur.entries {
			if cur.entries[j].key == seg {
				idx = j
				break
			}
		}
		if last {
			nv := scalarValue(value, existingKind(cur, idx))
			if idx >= 0 {
				cur.entries[idx].val = nv
			} else {
				cur.entries = append(cur.entries, phpEntry{key: seg, isInt: isIntKey(seg), val: nv})
			}
			return
		}
		if idx < 0 {
			child := &phpValue{kind: phpArray}
			cur.entries = append(cur.entries, phpEntry{key: seg, isInt: isIntKey(seg), val: child})
			cur = child
			continue
		}
		if cur.entries[idx].val.kind != phpArray {
			cur.entries[idx].val = &phpValue{kind: phpArray}
		}
		cur = cur.entries[idx].val
	}
}

func existingKind(parent *phpValue, idx int) phpKind {
	if idx < 0 {
		return phpString
	}
	return parent.entries[idx].val.kind
}

// scalarValue coerces a string update into the kind the file already used, so
// an int stays an int and a bool stays a bool. Anything that doesn't fit becomes
// a quoted string.
func scalarValue(value string, want phpKind) *phpValue {
	switch want {
	case phpInt:
		if _, err := strconv.Atoi(value); err == nil {
			return &phpValue{kind: phpInt, str: value}
		}
	case phpFloat:
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			return &phpValue{kind: phpFloat, str: value}
		}
	case phpBool:
		if value == "true" || value == "false" {
			return &phpValue{kind: phpBool, str: value}
		}
	}
	return &phpValue{kind: phpString, str: value}
}

func isIntKey(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func printValue(b *strings.Builder, v *phpValue, depth int) {
	switch v.kind {
	case phpArray:
		if len(v.entries) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		inner := strings.Repeat("    ", depth+1)
		for _, e := range v.entries {
			b.WriteString(inner)
			if e.isInt {
				b.WriteString(e.key)
			} else {
				b.WriteString("'" + escapeSingle(e.key) + "'")
			}
			b.WriteString(" => ")
			printValue(b, e.val, depth+1)
			b.WriteString(",\n")
		}
		b.WriteString(strings.Repeat("    ", depth) + "]")
	case phpInt, phpFloat, phpBool:
		b.WriteString(v.str)
	case phpNull:
		b.WriteString("null")
	default:
		b.WriteString("'" + escapeSingle(v.str) + "'")
	}
}

func escapeSingle(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// ── parser ──────────────────────────────────────────────────────────────────

type phpParser struct {
	src string
	pos int
}

// parsePhpReturn finds the top-level `return <value>;` and parses that value.
// Returns (nil, nil) when the file has no return statement.
func parsePhpReturn(src string) (*phpValue, error) {
	p := &phpParser{src: src}
	p.skipTrivia()
	for p.pos < len(p.src) {
		if p.hasWord("return") {
			p.pos += len("return")
			p.skipTrivia()
			return p.parseValue()
		}
		p.pos++
		p.skipTrivia()
	}
	return nil, nil
}

// hasWord reports whether the cursor sits on w as a standalone identifier.
func (p *phpParser) hasWord(w string) bool {
	if !strings.HasPrefix(p.src[p.pos:], w) {
		return false
	}
	if p.pos > 0 && isIdentByte(p.src[p.pos-1]) {
		return false
	}
	after := p.pos + len(w)
	return after >= len(p.src) || !isIdentByte(p.src[after])
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// skipTrivia consumes whitespace, the PHP open tag, and // # /* */ comments.
func (p *phpParser) skipTrivia() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		case strings.HasPrefix(p.src[p.pos:], "<?php"):
			p.pos += len("<?php")
		case strings.HasPrefix(p.src[p.pos:], "//"), c == '#':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		case strings.HasPrefix(p.src[p.pos:], "/*"):
			if end := strings.Index(p.src[p.pos+2:], "*/"); end >= 0 {
				p.pos += 2 + end + 2
			} else {
				p.pos = len(p.src)
			}
		default:
			return
		}
	}
}

func (p *phpParser) parseValue() (*phpValue, error) {
	p.skipTrivia()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected end of file")
	}
	switch c := p.src[p.pos]; {
	case c == '[':
		p.pos++
		return p.parseArrayBody(']')
	case p.hasWord("array"):
		p.pos += len("array")
		p.skipTrivia()
		if p.pos >= len(p.src) || p.src[p.pos] != '(' {
			return nil, fmt.Errorf("expected ( after array at %d", p.pos)
		}
		p.pos++
		return p.parseArrayBody(')')
	case c == '\'' || c == '"':
		s, err := p.parseString()
		return &phpValue{kind: phpString, str: s}, err
	case p.hasWord("true"), p.hasWord("false"):
		w := "true"
		if p.hasWord("false") {
			w = "false"
		}
		p.pos += len(w)
		return &phpValue{kind: phpBool, str: w}, nil
	case p.hasWord("null"):
		p.pos += len("null")
		return &phpValue{kind: phpNull}, nil
	case c == '-' || c == '.' || c >= '0' && c <= '9':
		start := p.pos
		if c == '-' {
			p.pos++
		}
		kind := phpInt
		for p.pos < len(p.src) {
			d := p.src[p.pos]
			if d == '.' || d == 'e' || d == 'E' || d == '+' || d == '-' {
				kind = phpFloat
			} else if d < '0' || d > '9' {
				break
			}
			p.pos++
		}
		return &phpValue{kind: kind, str: p.src[start:p.pos]}, nil
	}
	return nil, fmt.Errorf("unsupported value at offset %d", p.pos)
}

func (p *phpParser) parseArrayBody(closer byte) (*phpValue, error) {
	arr := &phpValue{kind: phpArray}
	next := 0
	for {
		p.skipTrivia()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unterminated array")
		}
		if p.src[p.pos] == closer {
			p.pos++
			return arr, nil
		}
		first, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		p.skipTrivia()
		entry := phpEntry{}
		if strings.HasPrefix(p.src[p.pos:], "=>") {
			p.pos += 2
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			entry.key = scalarString(first)
			entry.isInt = first.kind == phpInt
			entry.val = val
		} else {
			// Positional entry: synthesise the numeric key PHP would assign.
			entry.key = strconv.Itoa(next)
			entry.isInt = true
			entry.val = first
			next++
		}
		arr.entries = append(arr.entries, entry)
		p.skipTrivia()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
		}
	}
}

func (p *phpParser) parseString() (string, error) {
	quote := p.src[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			n := p.src[p.pos+1]
			switch {
			case n == quote || n == '\\':
				b.WriteByte(n)
				p.pos += 2
				continue
			case quote == '"' && n == 'n':
				b.WriteByte('\n')
				p.pos += 2
				continue
			case quote == '"' && n == 't':
				b.WriteByte('\t')
				p.pos += 2
				continue
			}
		}
		if c == quote {
			p.pos++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.pos++
	}
	return "", fmt.Errorf("unterminated string")
}

// Reader returns a key lookup for an env file in the given format ("dotenv",
// "php-const", "php-array"). An unreadable file yields a reader that returns
// empty strings, so callers need no error path for a missing env file.
func Reader(path, format string) func(key string) string {
	var (
		values map[string]string
		err    error
	)
	switch format {
	case "php-const":
		values, err = ReadPhpConst(path)
	case "php-array":
		values, err = ReadPhpArray(path)
	default:
		return func(key string) string { return ReadKey(path, key) }
	}
	if err != nil {
		return func(string) string { return "" }
	}
	return func(key string) string { return values[key] }
}
