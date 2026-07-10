package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFrameworkRequiresParsesFromYAML(t *testing.T) {
	var fw Framework
	src := "name: magento\nversion: \"2\"\nrequires:\n  - opensearch\n"
	if err := yaml.Unmarshal([]byte(src), &fw); err != nil {
		t.Fatal(err)
	}
	if len(fw.Requires) != 1 || fw.Requires[0] != "opensearch" {
		t.Fatalf("got %v", fw.Requires)
	}
}

func TestFrameworkRequiresAbsentIsNil(t *testing.T) {
	var fw Framework
	if err := yaml.Unmarshal([]byte("name: laravel\n"), &fw); err != nil {
		t.Fatal(err)
	}
	if fw.Requires != nil {
		t.Fatalf("got %v, want nil", fw.Requires)
	}
}

// A .lerd.yaml is untrusted. A required service pulls a container image and
// starts it, so an embedded framework_def must not be able to drive that, the
// same way it cannot drive host workers, commands, or nginx config.
func TestSanitizeProjectFrameworkDefStripsRequires(t *testing.T) {
	def := &Framework{Name: "evil", Requires: []string{"opensearch"}}
	safe := SanitizeProjectFrameworkDef(def)
	if safe.Requires != nil {
		t.Fatalf("requires survived sanitize: %v", safe.Requires)
	}
	if def.Requires == nil {
		t.Fatal("sanitize mutated the caller's definition")
	}
}
