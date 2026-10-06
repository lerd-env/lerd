package cli

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16le(s string, bom bool) []byte {
	var b bytes.Buffer
	if bom {
		b.Write([]byte{0xFF, 0xFE})
	}
	for _, c := range utf16.Encode([]rune(s)) {
		_ = binary.Write(&b, binary.LittleEndian, c)
	}
	return b.Bytes()
}

func TestWindowsVersionLabel(t *testing.T) {
	for _, c := range []struct {
		product, edition, display, build string
		ubr                              uint64
		want                             string
	}{
		// The registry still says Windows 10 on 11; the build number is the truth.
		{"Windows 10 Pro", "Professional", "25H2", "26200", 6899, "Windows 11 Pro 25H2 (build 26200.6899, edition Professional)"},
		{"Windows 10 Home", "Core", "22H2", "19045", 5965, "Windows 10 Home 22H2 (build 19045.5965, edition Core)"},
		{"", "", "", "", 0, "(unreadable)"},
	} {
		if got := windowsVersionLabel(c.product, c.edition, c.display, c.build, c.ubr); got != c.want {
			t.Errorf("windowsVersionLabel(%q) = %q, want %q", c.product, got, c.want)
		}
	}
}

// wsl.exe writes UTF-16LE to a pipe, with or without a BOM.
func TestDecodeConsoleText(t *testing.T) {
	want := "WSL version: 2.6.1.0\nKernel version: 6.6.87.2-1"
	for name, in := range map[string][]byte{
		"utf16 with bom": utf16le(want, true),
		"utf16 no bom":   utf16le(want, false),
		"utf8":           []byte(want),
	} {
		if got := decodeConsoleText(in); got != want {
			t.Errorf("%s: decodeConsoleText = %q, want %q", name, got, want)
		}
	}
}

func TestSummarizeMachineList(t *testing.T) {
	js := `[{"Name":"podman-machine-default","Default":true,"Running":true,"Starting":false,"LastUp":"2026-10-06T09:44:12+02:00","VMType":"hyperv","CPUs":8,"Memory":"3221225472","DiskSize":"107374182400","UserModeNetworking":false}]`
	got := summarizeMachineList([]byte(js))
	for _, want := range []string{"podman-machine-default (default)", "provider hyperv", "running", "8 CPUs", "3 GiB memory", "100 GiB disk", "user-mode networking off"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
	if got := summarizeMachineList([]byte(`[]`)); got != "(no podman machine)" {
		t.Errorf("empty list = %q", got)
	}
	if got := summarizeMachineList([]byte(`not json`)); !strings.HasPrefix(got, "(unparseable") {
		t.Errorf("bad json = %q", got)
	}
}

func TestDumpWindowsLogs_tailsInfraLogsAndSkipsContent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("lerd-ui.log", "old\nLerd UI listening\n")
	write("p9-serve.log", "lerd p9-guard: the 9p server exited with code 1\n")
	write("lerd-queue-shop.log", "site worker output\n")
	write("podman-install.log", string(utf16le("MSI (s) Product: Podman -- Installation failed.\r\n", true)))

	var buf bytes.Buffer
	dumpWindowsLogs(&buf, dir, 1, newLogFilter())
	out := buf.String()
	for _, want := range []string{"lerd-ui.log", "Lerd UI listening", "p9-serve.log", "9p server exited", "podman-install.log", "Installation failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "old") {
		t.Error("tail should keep only the last line")
	}
	if strings.Contains(out, "site worker") {
		t.Error("per-site worker logs must stay out of the report")
	}
}

func TestDumpWindowsLogs_missingDir(t *testing.T) {
	var buf bytes.Buffer
	dumpWindowsLogs(&buf, filepath.Join(t.TempDir(), "nope"), 10, newLogFilter())
	if !strings.Contains(buf.String(), "no log directory") {
		t.Errorf("got %q", buf.String())
	}
}
