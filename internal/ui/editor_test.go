package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// isolateEditorEnv pins HOME and the XDG dirs to throwaway temp dirs so the
// editor handler's home-confinement check and config lookup never touch the
// real environment. Returns the isolated HOME.
func isolateEditorEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return home
}

func writeEditorConfig(t *testing.T, editor string) {
	t.Helper()
	cfgFile := config.GlobalConfigFile()
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(cfgFile, []byte("editor: \""+editor+"\"\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestEditorCommandConfigTemplate(t *testing.T) {
	tests := []struct {
		name   string
		editor string
		file   string
		line   int
		want   []string
	}{
		{
			name:   "file and line placeholders substituted",
			editor: "phpstorm --line {line} {file}",
			file:   "/home/u/app/Models/User.php",
			line:   42,
			want:   []string{"phpstorm", "--line", "42", "/home/u/app/Models/User.php"},
		},
		{
			name:   "only file placeholder substituted",
			editor: "myeditor {file}",
			file:   "/home/u/a.php",
			line:   9,
			want:   []string{"myeditor", "/home/u/a.php"},
		},
		{
			name:   "no placeholder appends the file",
			editor: "myeditor -w",
			file:   "/home/u/a.php",
			line:   7,
			want:   []string{"myeditor", "-w", "/home/u/a.php"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateEditorEnv(t)
			writeEditorConfig(t, tc.editor)
			got := editorCommand(tc.file, tc.line)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("editorCommand() = %v, want %v", got, tc.want)
			}
		})
	}
}

func postEditor(t *testing.T, remoteAddr, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/open-editor", bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	rr := httptest.NewRecorder()
	handleOpenEditor(rr, req)
	return rr
}

func TestHandleOpenEditorRejects(t *testing.T) {
	isolateEditorEnv(t)

	t.Run("non-POST is 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/open-editor", nil)
		req.RemoteAddr = "127.0.0.1:5000"
		rr := httptest.NewRecorder()
		handleOpenEditor(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rr.Code)
		}
	})

	t.Run("non-loopback is 403", func(t *testing.T) {
		rr := postEditor(t, "8.8.8.8:5000", `{"path":"/etc/passwd","line":1}`)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
	})

	t.Run("invalid json is 400", func(t *testing.T) {
		rr := postEditor(t, "127.0.0.1:5000", `{not json`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("relative path is 400", func(t *testing.T) {
		rr := postEditor(t, "127.0.0.1:5000", `{"path":"app/User.php","line":1}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("path outside home is 403", func(t *testing.T) {
		rr := postEditor(t, "127.0.0.1:5000", `{"path":"/etc/passwd","line":1}`)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
	})

	t.Run("traversal escaping home is 403", func(t *testing.T) {
		// filepath.Clean collapses ../ so this resolves outside HOME.
		home := os.Getenv("HOME")
		rr := postEditor(t, "127.0.0.1:5000", `{"path":"`+home+`/../../etc/passwd","line":1}`)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
	})

	t.Run("missing file is 404", func(t *testing.T) {
		home := os.Getenv("HOME")
		rr := postEditor(t, "127.0.0.1:5000", `{"path":"`+home+`/nope.php","line":1}`)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})

	t.Run("directory is 404", func(t *testing.T) {
		dir := filepath.Join(os.Getenv("HOME"), "sub")
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		rr := postEditor(t, "127.0.0.1:5000", `{"path":"`+dir+`","line":1}`)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})
}

func TestHandleOpenEditorLaunches(t *testing.T) {
	home := isolateEditorEnv(t)
	// `true` is a harmless no-op stand-in for an editor; it exists on PATH and
	// exits 0, so the handler should reach the launch path and return 204.
	writeEditorConfig(t, "true")
	file := filepath.Join(home, "User.php")
	if err := os.WriteFile(file, []byte("<?php\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rr := postEditor(t, "127.0.0.1:5000", `{"path":"`+file+`","line":12}`)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body: %s)", rr.Code, rr.Body.String())
	}
}
