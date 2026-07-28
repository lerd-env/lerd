package serviceops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePodman puts a recording stub on PATH ahead of the real binary and returns
// the file its argv lands in. PodmanBin resolves fresh on every call, so this
// intercepts whatever the code under test shells out to.
func fakePodman(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + log + "\ncat > /dev/null 2>&1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func recordedArgv(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the command under test never shelled out to podman: %v", err)
	}
	return strings.Join(strings.Split(strings.TrimSpace(string(b)), "\n"), " ")
}

const clientImageEngine = `name: myengine
image: example/engine:1
introspect:
  entities:
    - kind: databases
      list: list-cmd
      format: sql
      image: example/client:1
      env:
        - CLIENT_TOKEN=abc
      actions:
        export: dump-cmd {{name}}
        import: load-cmd {{name}}
`

// An engine whose own image ships no client tooling declares the image its
// commands run in. Export honours it, so import has to as well: an import that
// execs inside the service container runs a binary that is not there.
func TestImportDatabaseRunsInTheDeclaredClientImage(t *testing.T) {
	writeCustomService(t, "myengine", clientImageEngine)
	log := fakePodman(t)

	if _, err := ImportDatabase("myengine", "shop", strings.NewReader("SELECT 1;\n"), ImportOptions{}); err != nil {
		t.Fatalf("import: %v", err)
	}
	argv := recordedArgv(t, log)

	if !strings.Contains(argv, "example/client:1") {
		t.Errorf("import ignored the declared client image\nargv: %s", argv)
	}
	if !strings.Contains(argv, "CLIENT_TOKEN=abc") {
		t.Errorf("import ignored the declared entity env\nargv: %s", argv)
	}
	if strings.Contains(argv, "lerd-myengine") {
		t.Errorf("import exec'd inside the service container instead of the client image\nargv: %s", argv)
	}
}

// The comparison that makes the asymmetry concrete: the same declaration, the
// same engine, and export already does the right thing.
func TestExportDatabaseRunsInTheDeclaredClientImage(t *testing.T) {
	writeCustomService(t, "myengine", clientImageEngine)
	log := fakePodman(t)

	if err := ExportDatabase("myengine", "shop", os.NewFile(0, os.DevNull)); err != nil {
		t.Fatalf("export: %v", err)
	}
	argv := recordedArgv(t, log)

	if !strings.Contains(argv, "example/client:1") {
		t.Errorf("export ignored the declared client image\nargv: %s", argv)
	}
}
