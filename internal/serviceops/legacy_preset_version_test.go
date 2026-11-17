package serviceops

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A store preset that used to pin one image and now lists versions, with 7 as
// the canonical one the old single image was.
func writeVersionedDocDB(t *testing.T) {
	t.Helper()
	dir := config.StorePresetsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir preset dir: %v", err)
	}
	body := "name: docdb\n" +
		"description: doc db\n" +
		"default_version: \"7\"\n" +
		"versions:\n" +
		"  - tag: \"7\"\n    image: docker.io/library/docdb:7\n    canonical: true\n    host_port: 27017\n" +
		"  - tag: \"9\"\n    image: docker.io/library/docdb:9\n    host_port: 27017\n" +
		"ports:\n  - \"{{host_port}}:27017\"\n" +
		"env_vars:\n  - DOC_HOST=lerd-{{name}}\n" +
		"connection_url: doc://127.0.0.1:{{host_port}}/\n"
	if err := os.WriteFile(filepath.Join(dir, "docdb.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}
}

// What a lerd install of the single-image preset wrote: no preset_version.
func legacyDocDB() *config.CustomService {
	return &config.CustomService{
		Name:          "docdb",
		Preset:        "docdb",
		Image:         "docker.io/library/docdb:7",
		Ports:         []string{"27017:27017"},
		EnvVars:       []string{"DOC_HOST=lerd-docdb"},
		ConnectionURL: "doc://127.0.0.1:27017/",
	}
}

func TestCaptureReinstallSpec_singleImageInstallReinstallsAsCanonical(t *testing.T) {
	reconcileEnv(t)
	writeVersionedDocDB(t)
	if err := config.SaveCustomService(legacyDocDB()); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}

	spec, err := captureReinstallSpec("docdb")
	if err != nil {
		t.Fatalf("a single-image install must still reinstall once its preset has versions: %v", err)
	}
	if spec.version != "7" || spec.image != "docker.io/library/docdb:7" {
		t.Errorf("reinstall must stay on the canonical version and running image, got %+v", spec)
	}
}

func TestCaptureReinstallSpec_singleImageInstallOffTheCanonicalLineRefused(t *testing.T) {
	reconcileEnv(t)
	writeVersionedDocDB(t)
	svc := legacyDocDB()
	svc.Image = "docker.io/library/docdb:6"
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}

	if _, err := captureReinstallSpec("docdb"); err == nil {
		t.Fatal("an install not running the canonical image must not be guessed onto it")
	}
}

func TestRefreshPresetDefinition_recordsTheCanonicalVersionOnASingleImageInstall(t *testing.T) {
	reconcileEnv(t)
	writeVersionedDocDB(t)
	installed := legacyDocDB()

	fresh, changed := refreshPresetDefinition(installed)
	if !changed || fresh.PresetVersion != "7" {
		t.Fatalf("the refresh must record version 7 once, got changed=%v version=%q", changed, fresh.PresetVersion)
	}
	if fresh.Name != "docdb" || fresh.Image != installed.Image {
		t.Errorf("name and image must not move, got %q %q", fresh.Name, fresh.Image)
	}
	if !slices.Equal(fresh.Ports, installed.Ports) || fresh.ConnectionURL != installed.ConnectionURL {
		t.Errorf("ports and connection must not move, got %v %q", fresh.Ports, fresh.ConnectionURL)
	}
	if !slices.Equal(fresh.EnvVars, installed.EnvVars) {
		t.Errorf("env must still name the bare service, got %v", fresh.EnvVars)
	}

	if _, again := refreshPresetDefinition(fresh); again {
		t.Error("once recorded, the next refresh must be a no-op")
	}
}

func TestRealReinstallInstall_addOnKeepsTheRunningImage(t *testing.T) {
	var image string
	prev := reinstallStreamingFn
	reinstallStreamingFn = func(name, version, img string, emit func(PhaseEvent)) (*config.CustomService, error) {
		image = img
		return &config.CustomService{Name: name}, nil
	}
	t.Cleanup(func() { reinstallStreamingFn = prev })

	spec := reinstallSpec{presetName: "docdb", version: "7", image: "docker.io/library/docdb:7.0.48"}
	if _, err := realReinstallInstall("docdb", spec, func(PhaseEvent) {}); err != nil {
		t.Fatalf("realReinstallInstall: %v", err)
	}
	if image != spec.image {
		t.Errorf("reinstall must install the image it captured, got %q", image)
	}
}

func TestResolvePresetOnImage_usesTheCapturedImage(t *testing.T) {
	reconcileEnv(t)
	writeVersionedDocDB(t)

	svc, err := resolvePresetOnImage("docdb", "7", "docker.io/library/docdb:7.0.48")
	if err != nil {
		t.Fatalf("resolvePresetOnImage: %v", err)
	}
	if svc.Image != "docker.io/library/docdb:7.0.48" || svc.Name != "docdb" {
		t.Errorf("want docdb on the captured image, got %q on %q", svc.Name, svc.Image)
	}
	if svc, _ = resolvePresetOnImage("docdb", "7", ""); svc.Image != "docker.io/library/docdb:7" {
		t.Errorf("with nothing captured the preset image stands, got %q", svc.Image)
	}
}
