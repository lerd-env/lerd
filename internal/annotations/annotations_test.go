package annotations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_AddsListsAndResolves(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a, err := Add(Annotation{Site: "shop", URL: "https://shop.test/cart", Selector: "#checkout", Comment: "Button is misaligned"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.Status != StatusOpen || a.Created == "" {
		t.Fatalf("added %+v", a)
	}
	open, _ := List("shop", StatusOpen)
	if len(open) != 1 || open[0].Comment != "Button is misaligned" {
		t.Fatalf("open %+v", open)
	}
	if _, err := Resolve("shop", a.ID, "Fixed the flex gap"); err != nil {
		t.Fatal(err)
	}
	if open, _ := List("shop", StatusOpen); len(open) != 0 {
		t.Errorf("still open: %+v", open)
	}
	all, _ := List("shop", "")
	if len(all) != 1 || all[0].Status != StatusResolved || all[0].Resolution != "Fixed the flex gap" || all[0].Resolved == "" {
		t.Errorf("resolved %+v", all)
	}
}

func TestStore_UpdatesAndDeletes(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a, _ := Add(Annotation{Site: "shop", Selector: "h1", Comment: "typo"})
	if _, err := Update("shop", a.ID, "typo in the heading"); err != nil {
		t.Fatal(err)
	}
	got, _ := Get("shop", a.ID)
	if got.Comment != "typo in the heading" {
		t.Errorf("updated %+v", got)
	}
	if err := Delete("shop", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Get("shop", a.ID); err == nil {
		t.Error("deleted annotation still found")
	}
}

func TestStore_RefusesAnEmptyNoteAndAnUnsafeSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if _, err := Add(Annotation{Site: "shop", Selector: "h1"}); err == nil {
		t.Error("empty comment accepted")
	}
	if _, err := Add(Annotation{Site: "../etc", Selector: "h1", Comment: "x"}); err == nil {
		t.Error("unsafe site name accepted")
	}
	if _, err := Get("shop", "../../x"); err == nil {
		t.Error("unsafe id accepted")
	}
	if matches, _ := filepath.Glob(filepath.Join(os.Getenv("XDG_DATA_HOME"), "..", "etc*")); len(matches) > 0 {
		t.Error("wrote outside the store")
	}
}
