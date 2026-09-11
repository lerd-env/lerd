package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/feedback"
	"gopkg.in/yaml.v3"
)

// LocalOverrideFile is the untracked companion to .lerd.yaml. Keys set here win
// over the committed file, so a temporary worktree can pin a domain, isolate its
// database or switch PHP without that choice ending up in the repo. Git-ignore it.
const LocalOverrideFile = ".lerd.local.yaml"

// LocalOverridePath returns the path of dir's local override file, whether or
// not it exists.
func LocalOverridePath(dir string) string {
	return filepath.Join(dir, LocalOverrideFile)
}

// LocalOverrideKeys returns the top-level keys the local override file sets, in
// alphabetical order, or nil when the file is absent.
func LocalOverrideKeys(dir string) ([]string, error) {
	root, err := loadMapping(LocalOverridePath(dir))
	if err != nil || root == nil {
		return nil, err
	}
	keys := make([]string, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keys = append(keys, root.Content[i].Value)
	}
	sort.Strings(keys)
	return keys, nil
}

// loadMapping parses path as a YAML mapping. A missing file and an empty
// document both return (nil, nil); anything that isn't a mapping is an error,
// since a caller can only merge keys.
func loadMapping(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: must be a mapping of settings, got %s", path, nodeKindName(root.Kind))
	}
	return root, nil
}

func nodeKindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		return "a single value"
	default:
		return "something else"
	}
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMappingValue(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
}

func deleteMappingKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// restoreLocalOverrides rewrites out, the mapping about to be written to
// .lerd.yaml, so every key the local override file owns keeps the committed
// file's own value (or stays absent). Without it the first save from any code
// path would bake the temporary override into the repo.
func restoreLocalOverrides(dir string, out *yaml.Node) error {
	local, err := loadMapping(LocalOverridePath(dir))
	if err != nil || local == nil {
		return err
	}
	base, err := loadMapping(filepath.Join(dir, ".lerd.yaml"))
	if err != nil {
		return err
	}

	var overridden []string
	for i := 0; i+1 < len(local.Content); i += 2 {
		key := local.Content[i].Value
		saved := mappingValue(out, key)
		if saved != nil && !sameNode(saved, local.Content[i+1]) {
			overridden = append(overridden, key)
		}
		if kept := mappingValue(base, key); kept != nil {
			setMappingValue(out, key, kept)
		} else {
			deleteMappingKey(out, key)
		}
	}
	if len(overridden) > 0 {
		sort.Strings(overridden)
		warnLocalOverride(overridden)
	}
	return nil
}

// sameNode compares two YAML values structurally, which is enough to tell an
// ordinary save (the value came from the local file in the first place) from a
// change the local file is about to swallow.
func sameNode(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind || a.Value != b.Value || len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !sameNode(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}

func warnLocalOverride(keys []string) {
	feedback.Warn("%s sets %s and keeps winning, so the new value was not saved. Change it there instead.",
		LocalOverrideFile, strings.Join(keys, ", "))
}
