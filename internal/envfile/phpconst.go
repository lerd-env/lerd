package envfile

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ReadPhpConst reads a WordPress-style wp-config.php file and returns a map
// of the defined PHP constants (define('KEY', 'value') calls).
// Only string and numeric constants are captured; boolean/null defines are ignored.
func ReadPhpConst(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	// Matches: define( 'KEY', 'value' ) or define("KEY", "value") with optional whitespace
	re := regexp.MustCompile(`(?i)define\(\s*['"](\w+)['"]\s*,\s*['"]([^'"]*)['"]\s*\)`)
	for _, match := range re.FindAllSubmatch(data, -1) {
		key := string(match[1])
		val := string(match[2])
		result[key] = val
	}
	return result, nil
}

// ApplyPhpConstUpdates rewrites define() values in a PHP file for the given keys.
// Existing define() calls are updated in-place. Keys that don't exist in the file
// are appended before "/* That's all" comment, or at the end of the file if not found.
func ApplyPhpConstUpdates(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(data)
	remaining := make(map[string]string)
	for k, v := range updates {
		remaining[k] = v
	}

	// Replace existing defines in-place
	re := regexp.MustCompile(`(?i)(define\(\s*['"])(\w+)(['"]\s*,\s*['"])[^'"]*(['"][^)]*\))`)
	content = re.ReplaceAllStringFunc(content, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 5 {
			return match
		}
		key := parts[2]
		if val, ok := remaining[key]; ok {
			delete(remaining, key)
			return parts[1] + key + parts[3] + val + parts[4]
		}
		return match
	})

	// Append remaining (new) keys
	if len(remaining) > 0 {
		var newLines strings.Builder
		for k, v := range remaining {
			newLines.WriteString(fmt.Sprintf("define( '%s', '%s' );\n", k, v))
		}

		// Insert before "/* That's all" if present, otherwise before closing ?>  or at EOF
		insertMarker := "/* That's all"
		if idx := strings.Index(content, insertMarker); idx != -1 {
			content = content[:idx] + newLines.String() + content[idx:]
		} else if idx := strings.LastIndex(content, "?>"); idx != -1 {
			content = content[:idx] + newLines.String() + content[idx:]
		} else {
			content += "\n" + newLines.String()
		}
	}

	if content == string(data) {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}
