package mcp

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestPresetEntry_carriesDiscoveryMetadata pins the fields a preset declares for
// discovery onto the MCP payload. Without admin_for an assistant cannot answer
// "which dashboard administers this database", which is the whole reason the
// preset declares it: phpMyAdmin fronts mariadb as well as mysql, and it is not
// a depends_on relationship.
func TestPresetEntry_carriesDiscoveryMetadata(t *testing.T) {
	got := newPresetEntry(config.PresetMeta{
		Name:        "phpmyadmin",
		Description: "MySQL/MariaDB web admin",
		Category:    "admin",
		Icon:        "database",
		AdminFor:    []string{"mysql", "mariadb"},
		DependsOn:   []string{"mysql"},
	})

	if got.Category != "admin" {
		t.Errorf("category = %q, want admin", got.Category)
	}
	if got.Icon != "database" {
		t.Errorf("icon = %q, want database", got.Icon)
	}
	if len(got.AdminFor) != 2 || got.AdminFor[0] != "mysql" || got.AdminFor[1] != "mariadb" {
		t.Errorf("admin_for = %v, want [mysql mariadb]", got.AdminFor)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "mysql" {
		t.Errorf("depends_on = %v, want [mysql]", got.DependsOn)
	}
}
