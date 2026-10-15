package plugins

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold_RenamesAndStaysValid(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Scaffold("demo", dest); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	p, err := ReadPlugin(dest)
	if err != nil {
		t.Fatalf("a scaffolded plugin must validate: %v", err)
	}
	if p.Name != "demo" {
		t.Errorf("name = %q, want demo", p.Name)
	}
}

// The skeleton's own name must not survive anywhere a user reads it: a skill
// left saying hello_greet sends the assistant to a tool that does not exist.
func TestScaffold_RenamesEveryMention(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo-tool")
	if err := Scaffold("demo-tool", dest); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// bin/greet prints a greeting, which is "Hello" by purpose, not by name.
		if strings.Contains(string(data), "hello") || (!strings.Contains(path, "/bin/") && strings.Contains(string(data), "Hello")) {
			t.Errorf("%s still names the skeleton", strings.TrimPrefix(path, dest))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	skill, _ := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if !strings.Contains(string(skill), "`demo-tool_greet`") {
		t.Errorf("the skill should name the plugin's own tool, got:\n%s", skill)
	}
	p, err := ReadPlugin(dest)
	if err != nil {
		t.Fatalf("a scaffolded plugin must validate: %v", err)
	}
	if p.UI.Page.Title != "Demo tool" {
		t.Errorf("page title = %q, want Demo tool", p.UI.Page.Title)
	}
}

func TestScaffold_RefusesExistingDirAndBadName(t *testing.T) {
	if err := Scaffold("demo", t.TempDir()); err == nil {
		t.Error("scaffolding over an existing dir must fail")
	}
	if err := Scaffold("Demo!", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("an invalid name must fail")
	}
}
