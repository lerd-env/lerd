package p9share

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/hugelgupf/p9/p9"
)

// The two failures that broke composer on a Hyper-V machine: replacing a file
// the server has looked up, and the host deleting it afterwards while the
// server still runs. Podman's p9 left a handle open on every lookup, so both
// were refused with "Access denied".
func TestServeReplacesALookedUpFileAndLetsTheHostDeleteIt(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"a.txt": "old", "b.txt": "new"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go serve(l, dir) //nolint:errcheck
	t.Cleanup(func() { l.Close() })

	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := p9.NewClient(conn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	root, err := client.Attach("/")
	if err != nil {
		t.Fatal(err)
	}

	_, target, err := root.Walk([]string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := target.GetAttr(p9.AttrMaskAll); err != nil {
		t.Fatal(err)
	}
	target.Close()

	if err := root.RenameAt("b.txt", root, "a.txt"); err != nil {
		t.Fatalf("renaming over a looked-up file: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "new" {
		t.Errorf("a.txt = %q after the rename, want new", got)
	}
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Errorf("the host cannot delete a file the server touched: %v", err)
	}
}

func TestServeRefusesAFolderThatIsNotThere(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := serve(l, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("serving a missing folder should fail")
	}
}
