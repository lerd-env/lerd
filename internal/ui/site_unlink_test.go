package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/cli"
)

func unlinkRequest(remote, query string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/sites/shop.test/unlink"+query, nil)
	r.RemoteAddr = remote
	return r
}

func TestUnlinkOptionsGuardDeletingTheDirectory(t *testing.T) {
	const local, lan = "127.0.0.1:50000", "192.168.1.20:50000"

	if opts, refusal := unlinkOptions(unlinkRequest(lan, ""), "shop"); refusal != "" || opts != (cli.UnlinkOptions{}) {
		t.Errorf("a plain unlink must pass through untouched, got %+v %q", opts, refusal)
	}
	if _, refusal := unlinkOptions(unlinkRequest(lan, "?delete_dir=1&confirm=shop"), "shop"); refusal == "" {
		t.Error("a request from another machine must not delete files")
	}
	if _, refusal := unlinkOptions(unlinkRequest(local, "?delete_dir=1&confirm=shoe"), "shop"); refusal == "" {
		t.Error("a mistyped name must not delete files")
	}
	opts, refusal := unlinkOptions(unlinkRequest(local, "?delete_dir=1&confirm=shop"), "shop")
	if refusal != "" || !opts.DeleteDir || !opts.Confirmed {
		t.Errorf("the host with the name typed back must delete, got %+v %q", opts, refusal)
	}
}

func TestUnlinkPreviewIsForTheHostOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/sites/shop.test/unlink:preview", nil)
	r.RemoteAddr = "192.168.1.20:50000"
	handleSiteUnlinkPreview(rec, r, "shop.test")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a request from another machine", rec.Code)
	}
}
