package mcp

import (
	"encoding/json"
	"testing"
)

// The MCP spec (and VSCode's validator) requires that every JSON Schema
// property with type: array also declare items, describing the element type.
// A missing items field causes VSCode to reject the tool with
// "tool parameters array type must have items." Issue #1265.
func TestToolList_arrayPropsHaveItems(t *testing.T) {
	tools := toolList()
	if len(tools) == 0 {
		t.Fatal("toolList returned no tools")
	}
	for _, tool := range tools {
		for name, prop := range tool.InputSchema.Properties {
			if prop.Type != "array" {
				continue
			}
			// Marshal the property to JSON and check for an items field.
			raw, err := json.Marshal(prop)
			if err != nil {
				t.Fatalf("marshal prop %s.%s: %v", tool.Name, name, err)
			}
			var check map[string]json.RawMessage
			if err := json.Unmarshal(raw, &check); err != nil {
				t.Fatalf("unmarshal prop %s.%s: %v", tool.Name, name, err)
			}
			if _, ok := check["items"]; !ok {
				t.Errorf("tool %q property %q has type array but no items field", tool.Name, name)
			}
		}
	}
}

// objectArrayProps are the array properties whose elements the handlers parse
// as objects rather than strings. Declaring string items on these is worse than
// declaring none: a strict client then sends strings, and the map[string]any
// assertion in the handler drops every element without reporting anything, so
// the framework saves with the section silently empty.
var objectArrayProps = map[string]bool{
	"setup": true,
	"logs":  true,
}

func TestToolList_arrayItemTypeMatchesHandler(t *testing.T) {
	for _, tool := range toolList() {
		for name, prop := range tool.InputSchema.Properties {
			if prop.Type != "array" {
				continue
			}
			if prop.Items == nil {
				continue // covered by TestToolList_arrayPropsHaveItems
			}
			want := "string"
			if objectArrayProps[name] {
				want = "object"
			}
			if prop.Items.Type != want {
				t.Errorf("tool %q property %q declares items type %q, handler parses %q",
					tool.Name, name, prop.Items.Type, want)
			}
		}
	}
}
