package envfile

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ChangedSince reports whether any key of the backup has another value in path,
// or is gone from it. Keys only path has do not count.
func ChangedSince(path, backup, format string) (bool, error) {
	old, err := snapshot(backup, format)
	if err != nil {
		return false, err
	}
	now, err := snapshot(path, format)
	if err != nil {
		return false, err
	}
	return len(differing(old, now)) > 0, nil
}

// RestoreValues puts the backup's values back into path, keeping every key
// added since. A file it cannot put back whole is refused and left as it was.
func RestoreValues(path, backup, format string) error {
	old, err := snapshot(backup, format)
	if err != nil {
		return err
	}
	now, err := snapshot(path, format)
	if err != nil {
		return err
	}
	var merged []byte
	if isDotenv(format) {
		merged, err = mergeDotenv(path, backup)
	} else {
		merged, err = mergePHP(path, format, differing(old, now))
	}
	if err != nil {
		return fmt.Errorf("%w; copy %s back by hand", err, backup)
	}
	if err := checkMerge(merged, format, old, now); err != nil {
		return fmt.Errorf("%w; copy %s back by hand", err, backup)
	}
	return writeFile(path, merged, 0o644)
}

func isDotenv(format string) bool { return format == "" || format == FormatDotenv }

// envValue is a key's value, or its source when only PHP can work it out.
type envValue struct {
	text string
	expr bool
}

func differing(old, now map[string]envValue) map[string]envValue {
	out := map[string]envValue{}
	for k, v := range old {
		if got, ok := now[k]; !ok || got != v {
			out[k] = v
		}
	}
	return out
}

// checkMerge holds a merge to both promises: every old value is back and every
// key added since is still there.
func checkMerge(merged []byte, format string, old, now map[string]envValue) error {
	tmp, err := tempFile(merged)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	after, err := snapshot(tmp, format)
	if err != nil {
		return err
	}
	for k := range differing(old, after) {
		return fmt.Errorf("%s cannot be put back", k)
	}
	for k, v := range now {
		if _, had := old[k]; !had && after[k] != v {
			return fmt.Errorf("putting the old values back would lose %s", k)
		}
	}
	return nil
}

func tempFile(data []byte) (string, error) {
	f, err := os.CreateTemp("", "lerd-env-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.Write(data)
	return f.Name(), err
}

// mergePHP runs the format's own writer on a copy, so a refused merge never
// touches the project.
func mergePHP(path, format string, changed map[string]envValue) ([]byte, error) {
	updates := map[string]string{}
	for k, v := range changed {
		if v.expr {
			return nil, fmt.Errorf("%s was a PHP expression", k)
		}
		updates[k] = v.text
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tmp, err := tempFile(data)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)
	if err := ApplyUpdatesIn(tmp, format, updates); err != nil {
		return nil, err
	}
	return os.ReadFile(tmp)
}

// snapshot reads every key of an env file, PHP expressions included, which the
// value readers leave out.
func snapshot(path, format string) (map[string]envValue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src, out := string(data), map[string]envValue{}
	switch format {
	case "", FormatDotenv:
		seen := map[string]int{}
		for _, b := range dotenvBlocks(src) {
			if b.key != "" {
				out[fmt.Sprintf("%s#%d", b.key, seen[b.key])] = envValue{text: b.value()}
				seen[b.key]++
			}
		}
	case FormatPhpArray:
		root, err := parsePhpReturn(src)
		if err != nil {
			return nil, err
		}
		if root != nil {
			collect("", root, out)
		}
	case FormatPhpVars:
		root := &phpValue{kind: phpArray}
		for _, a := range scanPhpAssignments(src) {
			setPathValue(root, a.path, a.value)
		}
		collect("", root, out)
	case FormatPhpConst:
		values, err := ReadPhpConst(path)
		if err != nil {
			return nil, err
		}
		for _, m := range phpDefine.FindAllStringSubmatch(src, -1) {
			if v, ok := values[m[1]]; ok {
				out[m[1]] = envValue{text: v}
			} else {
				out[m[1]] = envValue{text: m[2], expr: true}
			}
		}
	default:
		return nil, fmt.Errorf("env format %q is not one this version of lerd can read", format)
	}
	return out, nil
}

var phpDefine = regexp.MustCompile(`(?i)define\(\s*['"](\w+)['"]\s*,\s*(.*?)\s*\)\s*;`)

func collect(prefix string, v *phpValue, out map[string]envValue) {
	if v.kind != phpArray {
		out[prefix] = envValue{text: scalarString(v), expr: v.kind == phpExpr}
		return
	}
	for _, e := range v.entries {
		key := e.key
		if prefix != "" {
			key = prefix + "." + e.key
		}
		collect(key, e.val, out)
	}
}

// dotenvBlock is one entry with all its lines, or one line that is no entry.
type dotenvBlock struct{ key, text string }

func (b dotenvBlock) value() string {
	_, v, _ := strings.Cut(b.text, "=")
	return strings.Trim(strings.TrimSpace(v), `"'`)
}

func dotenvBlocks(src string) []dotenvBlock {
	lines := strings.SplitAfter(src, "\n")
	var out []dotenvBlock
	for i := 0; i < len(lines); i++ {
		k, v, ok := strings.Cut(lines[i], "=")
		k = strings.TrimPrefix(strings.TrimSpace(k), "export ")
		if !ok || strings.HasPrefix(k, "#") {
			out = append(out, dotenvBlock{text: lines[i]})
			continue
		}
		end := i + multiLineExtra(strings.TrimSpace(v), lines[i+1:])
		out = append(out, dotenvBlock{k, strings.Join(lines[i:end+1], "")})
		i = end
	}
	return out
}

// multiLineExtra counts the lines a quoted value runs on for, none when its
// quote closes on the first line or never.
func multiLineExtra(value string, rest []string) int {
	if value == "" || (value[0] != '"' && value[0] != '\'') || closesQuote(value[1:], value[0]) {
		return 0
	}
	for n, line := range rest {
		if closesQuote(line, value[0]) {
			return n + 1
		}
	}
	return 0
}

func closesQuote(s string, quote byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && quote == '"' {
			i++
		} else if s[i] == quote {
			return true
		}
	}
	return false
}

// mergeDotenv swaps each entry the backup has for the backup's own lines, and
// appends the ones that are gone.
func mergeDotenv(path, backup string) ([]byte, error) {
	now, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	was, err := os.ReadFile(backup)
	if err != nil {
		return nil, err
	}
	old := map[string][]dotenvBlock{}
	for _, b := range dotenvBlocks(string(was)) {
		if b.key != "" {
			old[b.key] = append(old[b.key], b)
		}
	}
	var out strings.Builder
	add := func(text string) {
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteString("\n")
		}
		out.WriteString(text)
	}
	for _, b := range dotenvBlocks(string(now)) {
		if left := old[b.key]; b.key != "" && len(left) > 0 {
			old[b.key] = left[1:]
			if left[0].value() != b.value() {
				b = left[0]
			}
		}
		add(b.text)
	}
	for _, b := range dotenvBlocks(string(was)) {
		if left := old[b.key]; b.key != "" && len(left) > 0 {
			old[b.key] = left[1:]
			add(left[0].text)
		}
	}
	return []byte(out.String()), nil
}
