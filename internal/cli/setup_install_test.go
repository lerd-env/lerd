package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func drupalInstall() *config.Framework {
	return &config.Framework{Install: &config.FrameworkInstall{
		Label:       "Install Drupal",
		Command:     "drush site:install",
		MissingFile: "web/sites/default/settings.php",
	}}
}

// A fresh project gets its framework's installer, ticked, ahead of the steps
// that need an installed site.
func TestFrameworkInstallStepOfferedWhileFileMissing(t *testing.T) {
	step, ok := frameworkInstallStep(t.TempDir(), drupalInstall())
	if !ok {
		t.Fatal("install step not offered on an uninstalled project")
	}
	if step.label != "Install Drupal" || !step.enabled {
		t.Errorf("got label %q enabled %v", step.label, step.enabled)
	}
}

// Once the file the installer writes exists, installing again would wipe the
// site, so the step is not offered at all.
func TestFrameworkInstallStepHiddenOnceInstalled(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "web", "sites", "default", "settings.php")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := frameworkInstallStep(dir, drupalInstall()); ok {
		t.Error("install step offered on an installed project")
	}
}

// Without the file to test for, there is no telling an installed project from a
// fresh one, so the declaration is refused rather than offered everywhere.
func TestFrameworkInstallStepRefusedWithoutMissingFile(t *testing.T) {
	fw := drupalInstall()
	fw.Install.MissingFile = ""
	if _, ok := frameworkInstallStep(t.TempDir(), fw); ok {
		t.Error("install step offered with no missing_file")
	}
	if _, ok := frameworkInstallStep(t.TempDir(), &config.Framework{}); ok {
		t.Error("install step offered for a framework that declares none")
	}
}
