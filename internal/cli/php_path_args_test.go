package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitPathArg(t *testing.T) {
	tests := []struct {
		name       string
		arg        string
		wantPrefix string
		wantPath   string
		wantOK     bool
	}{
		{"bare absolute path", "/tmp/x.php", "", "/tmp/x.php", true},
		{"long option value", "--config=/tmp/phpstan.neon", "--config=", "/tmp/phpstan.neon", true},
		{"relative path", "src/File.php", "", "", false},
		{"subcommand", "analyse", "", "", false},
		{"bare flag", "-c", "", "", false},
		{"long flag with no value", "--dry-run", "", "", false},
		{"long option with a non-path value", "--format=json", "", "", false},
		{"glued php -d never yields its value", "-derror_log=/tmp/x", "", "", false},
		{"a bare key=value is not an option", "error_log=/tmp/x", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, path, ok := splitPathArg(tt.arg)
			if prefix != tt.wantPrefix || path != tt.wantPath || ok != tt.wantOK {
				t.Errorf("splitPathArg(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.arg, prefix, path, ok, tt.wantPrefix, tt.wantPath, tt.wantOK)
			}
		})
	}
}

func TestStagePathArgIndexes(t *testing.T) {
	// Everything under /tmp is unreachable inside the container; the project
	// tree under /home is already mounted.
	needsStaging := func(p string) bool { return strings.HasPrefix(p, "/tmp/") }

	tests := []struct {
		name      string
		args      []string
		scriptIdx int
		want      []int
	}{
		{
			name:      "the file PhpStorm hands phpstan",
			args:      []string{"/home/u/app/vendor/bin/phpstan", "analyse", "--error-format=json", "/tmp/ide/File.php"},
			scriptIdx: 0,
			want:      []int{3},
		},
		{
			name:      "a temp config alongside the temp file",
			args:      []string{"/home/u/app/vendor/bin/php-cs-fixer", "fix", "--config=/tmp/ide/.php-cs-fixer.php", "/tmp/ide/File.php"},
			scriptIdx: 0,
			want:      []int{2, 3},
		},
		{
			name:      "a separate-value option value is a bare token",
			args:      []string{"/home/u/app/vendor/bin/phpstan", "analyse", "-c", "/tmp/ide/phpstan.neon"},
			scriptIdx: 0,
			want:      []int{3},
		},
		{
			name:      "paths the container already sees are left alone",
			args:      []string{"/home/u/app/vendor/bin/phpstan", "analyse", "/home/u/app/src"},
			scriptIdx: 0,
			want:      nil,
		},
		{
			name:      "php's own options are never staged",
			args:      []string{"-d", "error_log=/tmp/php.log", "/home/u/app/artisan", "migrate"},
			scriptIdx: 2,
			want:      nil,
		},
		{
			name:      "the script operand belongs to the /dev/stdin rescue",
			args:      []string{"/tmp/ide-phpinfo.php"},
			scriptIdx: 0,
			want:      nil,
		},
		{
			name:      "an invocation running no file has no arguments to stage",
			args:      []string{"-r", "echo 1;"},
			scriptIdx: -1,
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stagePathArgIndexes(tt.args, tt.scriptIdx, needsStaging)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stagePathArgIndexes(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// A staged path must not leak into the tool's output: an IDE keys the
// diagnostics it renders to the path it passed in, so the original has to come
// back out even when a chunk boundary falls inside the staged one.
func TestPathMapWriterRewritesAcrossWrites(t *testing.T) {
	items := []stagedPath{{orig: "/tmp/ide/File.php", staged: "/home/u/.local/share/lerd/stage/run-1/0/File.php"}}
	var out bytes.Buffer
	w := newPathMapWriter(&out, items)

	head := `{"files":{"/home/u/.local/share/lerd/sta`
	tail := `ge/run-1/0/File.php":{"errors":1}}}`
	if _, err := w.Write([]byte(head)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(tail)); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}

	want := `{"files":{"/tmp/ide/File.php":{"errors":1}}}`
	if got := out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// PHP's json_encode escapes every forward slash, so the JSON error formats an
// IDE asks these tools for carry the staged path in a shape a plain byte
// replacement never matches.
func TestPathMapWriterRewritesJSONEscapedPaths(t *testing.T) {
	items := []stagedPath{{orig: "/tmp/ide/File.php", staged: "/home/u/stage/run-1/0/File.php"}}
	var out bytes.Buffer
	w := newPathMapWriter(&out, items)
	if _, err := w.Write([]byte(`{"files":[{"path":"\/home\/u\/stage\/run-1\/0\/File.php"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	want := `{"files":[{"path":"\/tmp\/ide\/File.php"}]}`
	if got := out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestPathMapWriterPassesUnrelatedOutputThrough(t *testing.T) {
	var out bytes.Buffer
	w := newPathMapWriter(&out, []stagedPath{{orig: "/tmp/a.php", staged: "/home/u/stage/a.php"}})
	if _, err := w.Write([]byte("no paths here at all")); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "no paths here at all" {
		t.Errorf("output = %q", got)
	}
}

// php-cs-fixer and phpcbf rewrite the file they are given, so a staged copy the
// tool edited has to be copied back over the caller's own path.
func TestStageArgsSyncsAnEditedCopyBack(t *testing.T) {
	src := filepath.Join(t.TempDir(), "File.php")
	if err := os.WriteFile(src, []byte("<?php $x=1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()

	args := []string{"/home/u/app/vendor/bin/php-cs-fixer", "fix", src}
	stage, rewritten, err := stageArgs(args, []int{2}, root)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.remove()

	if rewritten[2] == src {
		t.Fatalf("argument was not rewritten: %v", rewritten)
	}
	if rewritten[0] != args[0] || rewritten[1] != args[1] {
		t.Errorf("untouched arguments changed: %v", rewritten)
	}
	if got, err := os.ReadFile(rewritten[2]); err != nil || string(got) != "<?php $x=1;" {
		t.Fatalf("staged copy = %q, %v", got, err)
	}
	if filepath.Base(rewritten[2]) != "File.php" {
		t.Errorf("staged copy lost its name: %s", rewritten[2])
	}

	if err := os.WriteFile(rewritten[2], []byte("<?php $x = 1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stage.syncBack(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<?php $x = 1;" {
		t.Errorf("original = %q, want the fixer's version", got)
	}
}

func TestStageArgsCopiesDirectoriesBothWays(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "A.php"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	stage, rewritten, err := stageArgs([]string{"phpcbf", srcDir}, []int{1}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer stage.remove()

	staged := filepath.Join(rewritten[1], "nested", "A.php")
	if got, err := os.ReadFile(staged); err != nil || string(got) != "a" {
		t.Fatalf("staged tree = %q, %v", got, err)
	}
	if err := os.WriteFile(staged, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stage.syncBack(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(srcDir, "nested", "A.php"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A" {
		t.Errorf("original = %q, want the edited version", got)
	}
}

// Staging is a copy, so a path far too large to be an IDE scratch file is
// refused with an explanation rather than duplicated.
func TestStageArgsRefusesAnOversizedPath(t *testing.T) {
	big := filepath.Join(t.TempDir(), "dump.sql")
	if err := os.WriteFile(big, make([]byte, stageSizeLimit+1), 0o644); err != nil {
		t.Fatal(err)
	}
	stage, _, err := stageArgs([]string{"import.php", big}, []int{1}, t.TempDir())
	if err == nil {
		stage.remove()
		t.Fatal("oversized path was staged, want an error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("mounts:")) {
		t.Errorf("error does not point at the way out: %v", err)
	}
}

func TestStageRemoveClearsTheScratchTree(t *testing.T) {
	src := filepath.Join(t.TempDir(), "File.php")
	if err := os.WriteFile(src, []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	stage, _, err := stageArgs([]string{"phpstan", src}, []int{1}, root)
	if err != nil {
		t.Fatal(err)
	}
	stage.remove()
	if _, err := os.Stat(stage.root); !os.IsNotExist(err) {
		t.Errorf("scratch tree survived removal: %v", err)
	}
}
