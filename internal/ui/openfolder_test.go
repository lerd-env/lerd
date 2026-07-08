package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func postFolder(t *testing.T, remoteAddr, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/open-folder", bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	rr := httptest.NewRecorder()
	handleOpenFolder(rr, req)
	return rr
}

func TestHandleOpenFolderRejects(t *testing.T) {
	home := isolateEditorEnv(t)

	t.Run("non-POST is 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/open-folder", nil)
		req.RemoteAddr = "127.0.0.1:5000"
		rr := httptest.NewRecorder()
		handleOpenFolder(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rr.Code)
		}
	})

	t.Run("non-loopback is 403", func(t *testing.T) {
		rr := postFolder(t, "8.8.8.8:5000", `{"path":"`+home+`"}`)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
	})

	t.Run("invalid json is 400", func(t *testing.T) {
		rr := postFolder(t, "127.0.0.1:5000", `{not json`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("relative path is 400", func(t *testing.T) {
		rr := postFolder(t, "127.0.0.1:5000", `{"path":"app"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("path outside home is 403", func(t *testing.T) {
		rr := postFolder(t, "127.0.0.1:5000", `{"path":"/etc"}`)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
	})

	t.Run("symlink under home pointing outside home is 403", func(t *testing.T) {
		link := filepath.Join(home, "escape")
		if err := os.Symlink("/etc", link); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		defer os.Remove(link)
		rr := postFolder(t, "127.0.0.1:5000", `{"path":"`+link+`"}`)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (symlink must not escape home)", rr.Code)
		}
	})

	t.Run("a file (not a dir) is 404", func(t *testing.T) {
		f := filepath.Join(home, "notadir.txt")
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		rr := postFolder(t, "127.0.0.1:5000", `{"path":"`+f+`"}`)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})
}
