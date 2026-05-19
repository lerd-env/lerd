package podman

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestCustomContainerName(t *testing.T) {
	if got := CustomContainerName("nestapp"); got != "lerd-custom-nestapp" {
		t.Errorf("CustomContainerName = %q, want lerd-custom-nestapp", got)
	}
}

func TestCustomImageName(t *testing.T) {
	if got := CustomImageName("nestapp"); got != "lerd-custom-nestapp:local" {
		t.Errorf("CustomImageName = %q, want lerd-custom-nestapp:local", got)
	}
}

func TestResolveContainerfile_Default(t *testing.T) {
	got := ResolveContainerfile("/srv/myapp", nil)
	want := "/srv/myapp/Containerfile.lerd"
	if got != want {
		t.Errorf("ResolveContainerfile(nil) = %q, want %q", got, want)
	}
}

func TestResolveContainerfile_EmptyConfig(t *testing.T) {
	cfg := &config.ContainerConfig{Port: 3000}
	got := ResolveContainerfile("/srv/myapp", cfg)
	want := "/srv/myapp/Containerfile.lerd"
	if got != want {
		t.Errorf("ResolveContainerfile(empty) = %q, want %q", got, want)
	}
}

func TestResolveContainerfile_Custom(t *testing.T) {
	cfg := &config.ContainerConfig{Port: 3000, Containerfile: "Containerfile"}
	got := ResolveContainerfile("/srv/myapp", cfg)
	want := "/srv/myapp/Containerfile"
	if got != want {
		t.Errorf("ResolveContainerfile(custom) = %q, want %q", got, want)
	}
}

func TestResolveBuildContext_Default(t *testing.T) {
	got := ResolveBuildContext("/srv/myapp", nil)
	if got != "/srv/myapp" {
		t.Errorf("ResolveBuildContext(nil) = %q, want /srv/myapp", got)
	}
}

func TestResolveBuildContext_Dot(t *testing.T) {
	cfg := &config.ContainerConfig{Port: 3000, BuildContext: "."}
	got := ResolveBuildContext("/srv/myapp", cfg)
	if got != "/srv/myapp" {
		t.Errorf("ResolveBuildContext(.) = %q, want /srv/myapp", got)
	}
}

func TestResolveBuildContext_Subdir(t *testing.T) {
	cfg := &config.ContainerConfig{Port: 3000, BuildContext: "docker"}
	got := ResolveBuildContext("/srv/myapp", cfg)
	want := "/srv/myapp/docker"
	if got != want {
		t.Errorf("ResolveBuildContext(docker) = %q, want %q", got, want)
	}
}

func TestHasContainerfile_Present(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM alpine\n"), 0644)
	if !HasContainerfile(dir) {
		t.Error("expected HasContainerfile = true")
	}
}

func TestContainerBaseImage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM node:20-alpine\nWORKDIR /app\n"), 0644)
	got := ContainerBaseImage(dir, nil)
	if got != "node:20-alpine" {
		t.Errorf("ContainerBaseImage = %q, want node:20-alpine", got)
	}
}

func TestContainerBaseImage_StripsDockerIO(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM docker.io/library/python:3.12-slim\n"), 0644)
	got := ContainerBaseImage(dir, nil)
	if got != "python:3.12-slim" {
		t.Errorf("ContainerBaseImage = %q, want python:3.12-slim", got)
	}
}

func TestContainerBaseImage_Missing(t *testing.T) {
	dir := t.TempDir()
	got := ContainerBaseImage(dir, nil)
	if got != "" {
		t.Errorf("ContainerBaseImage = %q, want empty", got)
	}
}

func TestContainerBaseImage_CustomPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM golang:1.23-alpine\n"), 0644)
	cfg := &config.ContainerConfig{Port: 8080, Containerfile: "Dockerfile"}
	got := ContainerBaseImage(dir, cfg)
	if got != "golang:1.23-alpine" {
		t.Errorf("ContainerBaseImage = %q, want golang:1.23-alpine", got)
	}
}

func TestHashContainerfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM node:20-alpine\n"), 0644)
	h1 := hashContainerfile(dir, nil)
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// Same content, same hash.
	h2 := hashContainerfile(dir, nil)
	if h1 != h2 {
		t.Errorf("same file should produce same hash: %q vs %q", h1, h2)
	}

	// Change content, different hash.
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM python:3.12\n"), 0644)
	h3 := hashContainerfile(dir, nil)
	if h3 == h1 {
		t.Error("different content should produce different hash")
	}
}

func TestHashContainerfile_Missing(t *testing.T) {
	dir := t.TempDir()
	h := hashContainerfile(dir, nil)
	if h != "" {
		t.Errorf("missing file should return empty hash, got %q", h)
	}
}

// Flipping container.target between stages of an unchanged Containerfile
// must invalidate the cache so CustomImageUpToDate rebuilds. Without this
// a `target: development` → `target: production` edit kept serving the
// dev image (issue #379).
func TestHashContainerfile_TargetChangeInvalidatesHash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte(
		"FROM alpine AS production\nFROM alpine AS development\n",
	), 0644)
	hDev := hashContainerfile(dir, &config.ContainerConfig{Port: 8080, Target: "development"})
	hProd := hashContainerfile(dir, &config.ContainerConfig{Port: 8080, Target: "production"})
	if hDev == "" || hProd == "" {
		t.Fatal("expected non-empty hashes")
	}
	if hDev == hProd {
		t.Errorf("hash must change when only Target flips; both = %q", hDev)
	}
	hNoTarget := hashContainerfile(dir, &config.ContainerConfig{Port: 8080})
	if hNoTarget == hDev || hNoTarget == hProd {
		t.Errorf("hash with no target must differ from both targeted variants")
	}
}

func TestStoreAndReadContainerfileHash(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM node:20\n"), 0644)

	StoreContainerfileHash("mysite", dir, nil)
	stored := readContainerfileHash("mysite")
	expected := hashContainerfile(dir, nil)
	if stored != expected {
		t.Errorf("stored hash %q != expected %q", stored, expected)
	}

	RemoveContainerfileHash("mysite")
	if got := readContainerfileHash("mysite"); got != "" {
		t.Errorf("hash should be empty after removal, got %q", got)
	}
}

func TestHasContainerfile_Absent(t *testing.T) {
	dir := t.TempDir()
	if HasContainerfile(dir) {
		t.Error("expected HasContainerfile = false")
	}
}

// Sn0wCrack's multi-stage Containerfile use case (issue #379): the target:
// field in .lerd.yaml's container block must translate to --target on the
// podman build invocation so the right stage gets built.
func TestBuildCustomImageArgs_TargetEmittedWhenSet(t *testing.T) {
	args := buildCustomImageArgs(
		"lerd-custom-acme:local",
		"/srv/acme/Containerfile.lerd",
		"/srv/acme",
		&config.ContainerConfig{Port: 3000, Target: "development"},
	)
	want := []string{"build", "-t", "lerd-custom-acme:local", "-f", "/srv/acme/Containerfile.lerd", "--target", "development", "/srv/acme"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v\nwant %v", args, want)
	}
}

func TestBuildCustomImageArgs_TargetOmittedWhenEmpty(t *testing.T) {
	args := buildCustomImageArgs(
		"lerd-custom-acme:local",
		"/srv/acme/Containerfile.lerd",
		"/srv/acme",
		&config.ContainerConfig{Port: 3000},
	)
	for _, a := range args {
		if a == "--target" {
			t.Errorf("--target must not appear when Target is empty; got %v", args)
		}
	}
}

func TestBuildCustomImageArgs_NilConfigSafe(t *testing.T) {
	args := buildCustomImageArgs("img:local", "/f", "/ctx", nil)
	want := []string{"build", "-t", "img:local", "-f", "/f", "/ctx"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v\nwant %v", args, want)
	}
}
