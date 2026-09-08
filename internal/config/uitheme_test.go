package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func themeDirFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := ThemesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir themes: %v", err)
	}
	return dir
}

func TestUIThemesReadsDirectory(t *testing.T) {
	dir := themeDirFixture(t)
	writeThemeFile(t, filepath.Join(dir, "ocean.yaml"), "name: Ocean\naccent: \"#3B7EA1\"\ncard: \"#141b1f\"\n")

	themes, errs := UIThemes()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(themes) != 1 {
		t.Fatalf("want 1 theme, got %d", len(themes))
	}
	got := themes[0]
	if got.ID != "ocean" || got.Name != "Ocean" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.Accent != "#3b7ea1" {
		t.Errorf("accent not normalised: %q", got.Accent)
	}
	if got.Card != "#141b1f" {
		t.Errorf("card = %q", got.Card)
	}
}

func TestUIThemesReportsBadFileWithoutDroppingTheRest(t *testing.T) {
	dir := themeDirFixture(t)
	writeThemeFile(t, filepath.Join(dir, "good.yaml"), "name: Good\naccent: \"#112233\"\n")
	writeThemeFile(t, filepath.Join(dir, "bad.yaml"), "name: Bad\naccent: rebeccapurple\n")

	themes, errs := UIThemes()
	if len(themes) != 1 || themes[0].ID != "good" {
		t.Fatalf("want only the good theme, got %+v", themes)
	}
	if len(errs) != 1 || errs[0].File != "bad.yaml" {
		t.Fatalf("want one described error, got %+v", errs)
	}
	// A CSS colour name is a colour, just not one that may pass, so the message
	// has to name the rule rather than only reject the value.
	if !strings.Contains(errs[0].Error, "only hex colours") || !strings.Contains(errs[0].Error, "rebeccapurple") {
		t.Errorf("error does not explain the rule: %q", errs[0].Error)
	}
}

func TestUIThemesRejectsUnusableName(t *testing.T) {
	dir := themeDirFixture(t)
	writeThemeFile(t, filepath.Join(dir, "Not Valid.yaml"), "name: X\naccent: \"#112233\"\n")

	themes, errs := UIThemes()
	if len(themes) != 0 {
		t.Fatalf("want no themes, got %+v", themes)
	}
	if len(errs) != 1 {
		t.Fatalf("want one error, got %+v", errs)
	}
}

func TestUIThemeMissingFieldsAreErrors(t *testing.T) {
	dir := themeDirFixture(t)
	writeThemeFile(t, filepath.Join(dir, "noname.yaml"), "accent: \"#112233\"\n")
	writeThemeFile(t, filepath.Join(dir, "noaccent.yaml"), "name: Nope\n")

	themes, errs := UIThemes()
	if len(themes) != 0 || len(errs) != 2 {
		t.Fatalf("themes=%+v errs=%+v", themes, errs)
	}
}

func TestSaveAndDeleteUITheme(t *testing.T) {
	themeDirFixture(t)

	if err := SaveUITheme("ocean", []byte("name: Ocean\naccent: \"#3b7ea1\"\n")); err != nil {
		t.Fatalf("save: %v", err)
	}
	themes, errs := UIThemes()
	if len(themes) != 1 || len(errs) != 0 {
		t.Fatalf("themes=%+v errs=%+v", themes, errs)
	}

	if err := DeleteUITheme("ocean"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	themes, _ = UIThemes()
	if len(themes) != 0 {
		t.Fatalf("want theme gone, got %+v", themes)
	}
}

func TestSaveUIThemeRefusesBadInput(t *testing.T) {
	themeDirFixture(t)

	if err := SaveUITheme("../escape", []byte("name: X\naccent: \"#112233\"\n")); err == nil {
		t.Error("want traversal rejected")
	}
	if err := SaveUITheme("ocean", []byte("name: X\naccent: url(evil)\n")); err == nil {
		t.Error("want non-hex accent rejected")
	}
	if err := DeleteUITheme("../../config"); err == nil {
		t.Error("want traversal rejected on delete")
	}
}

func TestUIThemesMissingDirectoryIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	themes, errs := UIThemes()
	if len(themes) != 0 || len(errs) != 0 {
		t.Fatalf("themes=%+v errs=%+v", themes, errs)
	}
}

func writeThemeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
