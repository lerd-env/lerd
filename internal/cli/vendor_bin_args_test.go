package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// frameworkWithVendorBinArgs installs a user framework definition under an
// isolated lerd home and returns a project directory that resolves to it.
// Written as a user framework rather than an embedded framework_def because
// the sanitiser strips vendor_bin_args from an untrusted project definition.
func frameworkWithVendorBinArgs(t *testing.T) string {
	t.Helper()
	isolateLerdHome(t)
	fwDir := config.FrameworksDir()
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := `name: wordpress
version: "6"
label: WordPress
vendor_bin_args:
  wp:
    - --allow-root
`
	if err := os.WriteFile(filepath.Join(fwDir, "wordpress.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: wordpress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVendorBinDefaultArgs_FromFramework(t *testing.T) {
	dir := frameworkWithVendorBinArgs(t)
	if got := vendorBinDefaultArgs(dir, "wp"); !reflect.DeepEqual(got, []string{"--allow-root"}) {
		t.Errorf("wp default args = %v, want [--allow-root]", got)
	}
	if got := vendorBinDefaultArgs(dir, "phpunit"); got != nil {
		t.Errorf("unrelated binary should get no default args, got %v", got)
	}
}

// An untrusted project definition must not be able to steer a binary the user
// asked for, so vendor_bin_args declared in .lerd.yaml is ignored.
func TestVendorBinDefaultArgs_IgnoresProjectDefinition(t *testing.T) {
	isolateLerdHome(t)
	dir := t.TempDir()
	body := `framework: wordpress
framework_def:
  name: wordpress
  version: "6"
  vendor_bin_args:
    wp:
      - --path=/etc
`
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := vendorBinDefaultArgs(dir, "wp"); got != nil {
		t.Errorf("project-declared args must be ignored, got %v", got)
	}
}

func TestVendorBinDefaultArgs_NoFramework(t *testing.T) {
	isolateLerdHome(t)
	if got := vendorBinDefaultArgs(t.TempDir(), "wp"); got != nil {
		t.Errorf("bare dir should get no default args, got %v", got)
	}
}

// The flag is a courtesy, not a mandate: a user who typed it stays in control
// and must not end up passing it twice.
func TestApplyVendorBinDefaults_SkipsWhenAlreadyPresent(t *testing.T) {
	got := applyVendorBinDefaults([]string{"--allow-root"}, []string{"core", "version", "--allow-root"})
	want := []string{"core", "version", "--allow-root"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// wp-cli reads global parameters ahead of the command, so defaults go in front.
func TestApplyVendorBinDefaults_PrependsOtherwise(t *testing.T) {
	got := applyVendorBinDefaults([]string{"--allow-root"}, []string{"core", "version"})
	want := []string{"--allow-root", "core", "version"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestApplyVendorBinDefaults_NoDefaults(t *testing.T) {
	got := applyVendorBinDefaults(nil, []string{"core", "version"})
	if !reflect.DeepEqual(got, []string{"core", "version"}) {
		t.Errorf("got %v", got)
	}
}
