package envfile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file handles the "php-vars" env format: a PHP file whose configuration
// is a series of top-level assignments to variables, as Drupal's settings.php
// is. Keys are addressed by a dotted path rooted at the variable name, so
// databases.default.default.host is $databases['default']['default']['host'].
//
// It is the shape a framework writes for itself. Drupal's installer appends the
// $databases array to settings.php and reads it back on every request, so lerd
// has to speak that file rather than leave connection values somewhere the
// application never looks. Writing rewrites only the statements it changes and
// leaves the rest of the file, which is mostly guidance the user may have
// edited, byte for byte.

// phpAssignment is one top-level `$var['a']['b'] = <value>;` statement, with
// the offsets of the value expression so it can be rewritten in place.
type phpAssignment struct {
	path     []string
	value    *phpValue
	rhsStart int
	rhsEnd   int
}

// ReadPhpVars parses a PHP file's top-level assignments and flattens them to
// dotted keys. A file with no assignments yields an empty map, not an error.
func ReadPhpVars(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	root := &phpValue{kind: phpArray}
	for _, a := range scanPhpAssignments(string(data)) {
		setPathValue(root, a.path, a.value)
	}
	flatten("", root, out)
	return out, nil
}

// ApplyPhpVarsUpdates sets each dotted key, rewriting the statement that owns it
// and appending a statement of its own for a key no assignment covers. A missing
// file (and its parent dirs) is created. An existing scalar keeps its type when
// the new value fits it.
func ApplyPhpVarsUpdates(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	original := string(data)
	src := original
	if src == "" {
		src = "<?php\n"
	}

	assignments := scanPhpAssignments(src)

	// Sorted so a file gains new statements in a stable order however the
	// updates were collected.
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Reprinting an assignment renders it in lerd's own style, so a statement
	// whose values already match is left exactly as the framework wrote it
	// rather than reformatted for nothing.
	before := make([]map[string]string, len(assignments))
	for i, a := range assignments {
		before[i] = map[string]string{}
		flatten("", a.value, before[i])
	}

	touched := map[int]bool{}
	var appended []string
	for _, key := range keys {
		segs := strings.Split(key, ".")
		if i, rest := ownerOf(assignments, segs); i >= 0 {
			if len(rest) == 0 {
				// The statement assigns this very path, so its value is the one
				// to replace: `$databases['default']['default']['host'] = 'x';`
				// has nothing below it to descend into.
				assignments[i].value = scalarValue(updates[key], assignments[i].value.kind)
			} else {
				setPath(assignments[i].value, rest, updates[key])
			}
			touched[i] = true
			continue
		}
		appended = append(appended, printAssignment(segs, updates[key]))
	}

	dirty := map[int]bool{}
	for i := range assignments {
		if !touched[i] {
			continue
		}
		after := map[string]string{}
		flatten("", assignments[i].value, after)
		dirty[i] = !sameValues(before[i], after)
	}

	// Rewritten back to front so each splice leaves the earlier offsets valid.
	out := src
	for i := len(assignments) - 1; i >= 0; i-- {
		if !dirty[i] {
			continue
		}
		var b strings.Builder
		printValue(&b, assignments[i].value, 0)
		out = out[:assignments[i].rhsStart] + b.String() + out[assignments[i].rhsEnd:]
	}
	if len(appended) > 0 {
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += strings.Join(appended, "")
	}

	// A file already holding every target value keeps its mtime, so an env sync
	// that changes nothing doesn't churn a file the user has open.
	if out == original {
		return nil
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return writeFile(path, []byte(out), 0o644)
}

func sameValues(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// ownerOf returns the assignment that owns a key, along with the segments left
// to set inside that assignment's value.
//
// PHP runs a file top to bottom, so where several assignments reach the same
// path the last one is what the application ends up with, whether it is a whole
// array replacing an earlier one or a leaf overriding it. Writing to any
// earlier one leaves the value shadowed: the file would say what lerd wrote and
// the application would go on using what came after it. So the match is the
// last, not the most specific.
func ownerOf(assignments []phpAssignment, segs []string) (int, []string) {
	best, bestLen := -1, -1
	for i, a := range assignments {
		if len(a.path) > len(segs) || a.value.kind != phpArray && len(a.path) != len(segs) {
			continue
		}
		match := true
		for j, seg := range a.path {
			if segs[j] != seg {
				match = false
				break
			}
		}
		if match {
			best, bestLen = i, len(a.path)
		}
	}
	if best < 0 {
		return -1, nil
	}
	return best, segs[bestLen:]
}

// printAssignment renders a statement for a key no assignment covers, assigning
// the leaf directly: PHP creates the arrays above it on the way.
func printAssignment(segs []string, value string) string {
	var b strings.Builder
	b.WriteString("$")
	b.WriteString(segs[0])
	for _, seg := range segs[1:] {
		b.WriteString("['")
		b.WriteString(escapeSingle(seg))
		b.WriteString("']")
	}
	b.WriteString(" = ")
	printValue(&b, scalarValue(value, phpString), 0)
	b.WriteString(";\n")
	return b.String()
}

// setPathValue grafts a parsed value onto the tree at segs, replacing whatever
// sat there. Assignments are applied in file order, so a later statement wins
// over an earlier one exactly as PHP would run them.
func setPathValue(root *phpValue, segs []string, v *phpValue) {
	cur := root
	for i, seg := range segs {
		idx := -1
		for j := range cur.entries {
			if cur.entries[j].key == seg {
				idx = j
				break
			}
		}
		if i == len(segs)-1 {
			if idx >= 0 {
				cur.entries[idx].val = v
			} else {
				cur.entries = append(cur.entries, phpEntry{key: seg, isInt: isIntKey(seg), val: v})
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

// scanPhpAssignments walks the source for `$var['a']['b'] = <value>;`
// statements. skipTrivia eats comments, so an assignment shown as an example
// inside one is never read as configuration, and a string literal is stepped
// over whole so a `$` inside it starts nothing.
func scanPhpAssignments(src string) []phpAssignment {
	var out []phpAssignment
	p := &phpParser{src: src}
	for {
		p.skipTrivia()
		if p.pos >= len(p.src) {
			return out
		}
		if c := p.src[p.pos]; c == '\'' || c == '"' {
			if _, err := p.parseString(); err != nil {
				return out
			}
			continue
		}
		if p.src[p.pos] != '$' {
			p.pos++
			continue
		}
		start := p.pos
		a, ok := p.parseAssignment()
		if !ok {
			p.pos = start + 1
			continue
		}
		out = append(out, a)
	}
}

// parseAssignment reads one assignment statement from the cursor. It returns
// false, leaving the cursor for the caller to reset, for anything that is not a
// plain `$var[...] = <value>;`: a read of a variable, a compound assignment, a
// comparison, or a value shape the parser doesn't model.
func (p *phpParser) parseAssignment() (phpAssignment, bool) {
	p.pos++ // the '$'
	nameStart := p.pos
	for p.pos < len(p.src) && isIdentByte(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == nameStart {
		return phpAssignment{}, false
	}
	path := []string{p.src[nameStart:p.pos]}

	for {
		p.skipTrivia()
		if p.pos >= len(p.src) || p.src[p.pos] != '[' {
			break
		}
		p.pos++
		p.skipTrivia()
		if p.pos >= len(p.src) || (p.src[p.pos] != '\'' && p.src[p.pos] != '"') {
			return phpAssignment{}, false
		}
		key, err := p.parseString()
		if err != nil {
			return phpAssignment{}, false
		}
		p.skipTrivia()
		if p.pos >= len(p.src) || p.src[p.pos] != ']' {
			return phpAssignment{}, false
		}
		p.pos++
		path = append(path, key)
	}

	p.skipTrivia()
	if p.pos >= len(p.src) || p.src[p.pos] != '=' {
		return phpAssignment{}, false
	}
	// '==', '=>' and the compound assignments are not this statement.
	if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '=' || p.src[p.pos+1] == '>') {
		return phpAssignment{}, false
	}
	p.pos++
	p.skipTrivia()

	rhsStart := p.pos
	value, err := p.parseValue()
	if err != nil {
		return phpAssignment{}, false
	}
	rhsEnd := p.pos
	p.skipTrivia()
	if p.pos >= len(p.src) || p.src[p.pos] != ';' {
		return phpAssignment{}, false
	}
	p.pos++

	return phpAssignment{path: path, value: value, rhsStart: rhsStart, rhsEnd: rhsEnd}, true
}
