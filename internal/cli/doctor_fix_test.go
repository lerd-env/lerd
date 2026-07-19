package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDoctorFixRejectsNonAuto(t *testing.T) {
	var buf bytes.Buffer
	if err := ApplyDoctorFix(nil, &buf); err == nil {
		t.Fatal("nil fix should error")
	}
	if err := ApplyDoctorFix(manualFix, &buf); err == nil {
		t.Fatal("manual fix should not be applied")
	}
}

func TestApplyDoctorFixUnknownKey(t *testing.T) {
	var buf bytes.Buffer
	if err := ApplyDoctorFix(&DoctorFix{Tier: FixAuto, Key: "bogus"}, &buf); err == nil {
		t.Fatal("unknown key should error")
	}
}

func TestApplyDoctorFixMkdirCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "quadlets")
	var buf bytes.Buffer
	if err := ApplyDoctorFix(autoFix(fixMkdir, dir, "create it"), &buf); err != nil {
		t.Fatalf("mkdir fix failed: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
	if !strings.Contains(buf.String(), dir) {
		t.Fatalf("output did not mention the target: %q", buf.String())
	}
}

func TestApplyDoctorFixMkdirEmptyArg(t *testing.T) {
	var buf bytes.Buffer
	if err := ApplyDoctorFix(autoFix(fixMkdir, "", "create it"), &buf); err == nil {
		t.Fatal("empty mkdir target should error")
	}
}

func TestHeavyFixClassification(t *testing.T) {
	if !IsHeavyFix(autoFix(fixCleanup, "", "")) || !IsHeavyFix(autoFix(fixInstall, "", "")) {
		t.Fatal("cleanup and install should be heavy")
	}
	if IsHeavyFix(autoFix(fixMkdir, "/x", "")) {
		t.Fatal("mkdir should not be heavy")
	}
}
