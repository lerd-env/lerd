package envfile

import "strings"

// MergeResult is the outcome of merging missing .env.example keys into an
// existing .env. Merged is the full proposed file content, ready to render in a
// diff or write to disk; Added lists the keys that were inserted, in the order
// they appear in .env.example; AddedLines gives the 1-based line numbers in
// Merged that are newly inserted (key lines and their carried comments), so a
// viewer can decorate exactly the added lines without re-diffing.
type MergeResult struct {
	Merged     string
	Added      []string
	AddedLines []int
}

// exampleEntry is one key line from .env.example along with the contiguous
// comment lines directly above it (its "attached" trivia). Comments separated
// from the key by a blank line are treated as a section header rather than a
// per-key note and are not attached, so they aren't duplicated across every key
// in the section when keys are inserted individually.
type exampleEntry struct {
	key      string
	line     string
	comments []string
}

// MergeMissing computes a proposed .env that adds the keys present in
// exampleContent but missing from envContent, placing each one next to the
// neighbours it has in .env.example rather than appending them all at the end.
//
// Placement is anchor-based: walking the example in order, the last example key
// that also exists in the .env is the current anchor, and a missing key is
// queued to be inserted right after that anchor's line. This keeps a missing
// DB_PORT inside the DB_* block instead of orphaning it at the bottom. Keys
// missing before any shared key land at the top, in example order.
//
// When include is non-nil, only missing keys for which include[key] is true are
// added; a nil include adds every missing key. Values, quoting, and attached
// comments are copied verbatim from the example so placeholders survive. Keys in
// the .env that are absent from the example are never touched.
func MergeMissing(exampleContent, envContent string, include map[string]bool) MergeResult {
	entries := parseExampleEntries(exampleContent)

	envLines, trailingNL := splitLines(envContent)
	present := presentKeyLines(envLines)

	// Group insertions by the env line index they anchor after (-1 = top),
	// preserving example order within each group and skipping duplicates.
	inserts := map[int][]exampleEntry{}
	var added []string
	seen := map[string]bool{}
	anchor := -1
	for _, e := range entries {
		if idx, ok := present[e.key]; ok {
			anchor = idx
			continue
		}
		if include != nil && !include[e.key] {
			continue
		}
		if seen[e.key] {
			continue
		}
		seen[e.key] = true
		inserts[anchor] = append(inserts[anchor], e)
		added = append(added, e.key)
	}

	if len(added) == 0 {
		return MergeResult{Merged: envContent}
	}

	var out []string
	var addedLines []int
	appendBlocks := func(es []exampleEntry) {
		for _, e := range es {
			for _, c := range e.comments {
				out = append(out, c)
				addedLines = append(addedLines, len(out))
			}
			out = append(out, e.line)
			addedLines = append(addedLines, len(out))
		}
	}
	appendBlocks(inserts[-1])
	for i, l := range envLines {
		out = append(out, l)
		appendBlocks(inserts[i])
	}

	merged := strings.Join(out, "\n")
	if trailingNL || envContent == "" {
		merged += "\n"
	}
	return MergeResult{Merged: merged, Added: added, AddedLines: addedLines}
}

// parseExampleEntries returns the key lines of an .env.example in order, each
// carrying the comment lines directly above it (broken by any blank line).
func parseExampleEntries(content string) []exampleEntry {
	lines, _ := splitLines(content)
	var entries []exampleEntry
	var attached []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		switch {
		case trimmed == "":
			attached = nil
		case strings.HasPrefix(trimmed, "#"):
			attached = append(attached, l)
		default:
			k, _, ok := strings.Cut(l, "=")
			if k = strings.TrimSpace(k); ok && k != "" {
				entries = append(entries, exampleEntry{key: k, line: l, comments: attached})
			}
			attached = nil
		}
	}
	return entries
}

// presentKeyLines maps each key in envLines to the index of its first
// occurrence, matching ReadKey's first-wins semantics.
func presentKeyLines(envLines []string) map[string]int {
	present := map[string]int{}
	for i, l := range envLines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		k, _, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		if k = strings.TrimSpace(k); k != "" {
			if _, exists := present[k]; !exists {
				present[k] = i
			}
		}
	}
	return present
}

// splitLines splits content into lines without a trailing empty element,
// reporting whether the content ended with a newline so callers can restore it.
func splitLines(content string) (lines []string, trailingNL bool) {
	if content == "" {
		return nil, false
	}
	trailingNL = strings.HasSuffix(content, "\n")
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n"), trailingNL
}
