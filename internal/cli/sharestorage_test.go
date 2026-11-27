package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// siteWithEnv is a project whose framework maps the bucket and its URL to
// the keys of its service definition, with env as its env file.
func siteWithEnv(t *testing.T, env string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return siteDirWithEnv(t, t.TempDir(), env)
}

func siteDirWithEnv(t *testing.T, dir, env string) string {
	t.Helper()
	for name, body := range map[string]string{"artisan": "", ".env": env} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// serveStorageOnDomain gives the rustfs service a domain of its own.
func serveStorageOnDomain(t *testing.T, domain string, port int) {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	sc := cfg.Services["rustfs"]
	sc.Domain, sc.PublishedPort = domain, port
	cfg.Services["rustfs"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestSiteShareStorage_readsALocalBucket(t *testing.T) {
	s := siteShareStorage(siteWithEnv(t, "AWS_BUCKET=shop\nAWS_URL=http://localhost:9000/shop\n"))
	if s == nil || s.bucket != "shop" || s.port != "9000" || s.host != "localhost:9000" {
		t.Fatalf("storage = %+v, want the shop bucket on port 9000", s)
	}
}

func TestSiteShareStorage_readsABucketOnTheServiceDomain(t *testing.T) {
	dir := siteWithEnv(t, "AWS_BUCKET=shop\nAWS_URL=\"https://rustfs.test/shop\"\n")
	serveStorageOnDomain(t, "rustfs.test", 9123)
	s := siteShareStorage(dir)
	if s == nil || s.bucket != "shop" || s.port != "9123" || s.host != "rustfs.test" {
		t.Fatalf("storage = %+v, want the shop bucket on the port rustfs.test is served from", s)
	}
}

// Store definitions fill the bucket with {{site}}, as do other services' vars.
func TestSiteShareStorage_readsTheStoreDefinitionsMapping(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	store := filepath.Join(data, "lerd", "frameworks")
	def := `name: acme
label: Acme
public_dir: public
detect:
    - file: acme.json
env:
    file: .env
    services:
        postgres:
            vars:
                - DB_DATABASE={{site}}
        rustfs:
            vars:
                - AWS_BUCKET={{site}}
                - AWS_URL=http://localhost:9000/{{site}}
`
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "acme.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{"acme.json": "{}", ".env": "DB_DATABASE=shop_db\nAWS_BUCKET=shop\nAWS_URL=http://localhost:9000/shop\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if s := siteShareStorage(dir); s == nil || s.bucket != "shop" || s.port != "9000" {
		t.Fatalf("storage = %+v, want the shop bucket on port 9000", s)
	}
}

func TestSiteShareStorage_leavesOtherStorageAlone(t *testing.T) {
	for name, env := range map[string]string{
		"no s3":          "FILESYSTEM_DISK=local\n",
		"real bucket":    "AWS_BUCKET=shop\nAWS_URL=https://shop.s3.amazonaws.com\n",
		"cdn in front":   "AWS_BUCKET=shop\nAWS_URL=https://cdn.example.com/shop\n",
		"unknown domain": "AWS_BUCKET=shop\nAWS_URL=https://rustfs.test/shop\n",
		"prefixed path":  "AWS_BUCKET=shop\nAWS_URL=http://localhost:9000/shop/media\n",
		"no bucket name": "AWS_URL=http://localhost:9000/shop\n",
		"other loopback": "AWS_BUCKET=shop\nAWS_URL=http://127.0.0.2:9000/shop\n",
	} {
		t.Run(name, func(t *testing.T) {
			if s := siteShareStorage(siteWithEnv(t, env)); s != nil {
				t.Errorf("storage = %+v, want none", s)
			}
		})
	}
}

func TestShareTargetFor_worktreeSharesItsOwnBucket(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, wt := makeWorktreeLayout(t, "rapids", "feature")
	siteDirWithEnv(t, parent, "AWS_BUCKET=rapids\nAWS_URL=http://localhost:9000/rapids\n")
	siteDirWithEnv(t, wt, "AWS_BUCKET=feature\nAWS_URL=http://localhost:9000/feature\n")
	site := &config.Site{Name: "rapids", Path: parent, Domains: []string{"rapids.test"}}

	target, err := shareTargetFor(site, "feature")
	if err != nil {
		t.Fatalf("shareTargetFor: %v", err)
	}
	if target.storage == nil || target.storage.bucket != "feature" {
		t.Errorf("storage = %+v, want the worktree's bucket", target.storage)
	}
	// a branch with no worktree must not be shared with the main site's bucket
	if _, err := shareTargetFor(site, "gone"); err == nil {
		t.Error("expected an error for a branch whose worktree is missing")
	}
}

// fakeRustFS stands in for lerd's RustFS and records what reached it.
func fakeRustFS(t *testing.T) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Clone(r.Context()))
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "jpeg bytes")
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func storageAt(t *testing.T, rawURL, bucket string) *shareStorage {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return &shareStorage{bucket: bucket, port: u.Port(), host: u.Host}
}

// sharedProxy shares a site through a tunnel together with its storage.
func sharedProxy(t *testing.T, site http.HandlerFunc, s *shareStorage) int {
	t.Helper()
	backend := httptest.NewServer(site)
	t.Cleanup(backend.Close)
	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false, s)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	t.Cleanup(stop)
	return port
}

func tunnelRequest(t *testing.T, port int, method, path string, header http.Header) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	req.Host = "abc.trycloudflare.com"
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

func TestStartHostProxy_pointsBucketURLsAtTheTunnel(t *testing.T) {
	rustfs, _ := fakeRustFS(t)
	s := storageAt(t, rustfs.URL, "shop")
	file := rustfs.URL + "/shop/public/cover.jpg"
	vite := `<script src="http://localhost:1/@vite/client">`
	port := sharedProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Link", "<"+file+">; rel=preload; as=image")
		w.Header().Set("Location", file)
		fmt.Fprintf(w, `<img src="%s"><div data-page="{&quot;cover&quot;:&quot;%s&quot;}"></div>%s`, file, escapeSlashes(file), vite)
	}, s)

	resp, body := tunnelRequest(t, port, "GET", "/", nil)

	shared := "https://abc.trycloudflare.com/__lerd_vite__/" + s.port + "/shop/public/cover.jpg"
	// any other loopback port stays closed on a public share, so its URLs stay put
	for _, want := range []string{`<img src="` + shared + `">`, escapeSlashes(shared), vite} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
	if link := resp.Header.Get("Link"); !strings.Contains(link, "<"+shared+">") {
		t.Errorf("Link = %q, want the tunnel URL", link)
	}
	if loc := resp.Header.Get("Location"); loc != shared {
		t.Errorf("Location = %q, want %q", loc, shared)
	}
}

func TestStartHostProxy_sharesABucketOnTheServiceDomain(t *testing.T) {
	rustfs, got := fakeRustFS(t)
	s := storageAt(t, rustfs.URL, "shop")
	s.host, s.domain = "rustfs.test", serviceDomainURLRe("rustfs.test")
	file := "https://rustfs.test/shop/cover.jpg"
	other := `<a href="https://rustfs.test.example.com/shop/cover.jpg">`
	port := sharedProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<img src="%s"><div data-page="{&quot;cover&quot;:&quot;%s&quot;}"></div>%s`, file, escapeSlashes(file), other)
	}, s)

	_, body := tunnelRequest(t, port, "GET", "/", nil)
	path := "/__lerd_vite__/" + s.port + "/shop/cover.jpg"
	shared := "https://abc.trycloudflare.com" + path
	for _, want := range []string{`<img src="` + shared + `">`, escapeSlashes(shared), other} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}

	if resp, _ := tunnelRequest(t, port, "GET", path, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want the file", resp.StatusCode)
	}
	// fetched from the loopback port, under the host a presigned URL was signed for
	if r := (*got)[0]; r.Host != "rustfs.test" || r.URL.Path != "/shop/cover.jpg" {
		t.Errorf("RustFS got %s%s, want rustfs.test/shop/cover.jpg", r.Host, r.URL.Path)
	}
}

func TestStartHostProxy_servesTheBucketsFiles(t *testing.T) {
	rustfs, got := fakeRustFS(t)
	s := storageAt(t, rustfs.URL, "shop")
	port := sharedProxy(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the site got %s, a bucket file is not its", r.URL.Path)
	}, s)

	resp, body := tunnelRequest(t, port, "GET", "/__lerd_vite__/"+s.port+"/shop/private/a%20b.jpg?X-Amz-Signature=abc",
		http.Header{"Cookie": {"session=secret"}, "Authorization": {"Bearer secret"}})

	if resp.StatusCode != http.StatusOK || body != "jpeg bytes" {
		t.Fatalf("got %d %q, want the file", resp.StatusCode, body)
	}
	r := (*got)[0]
	if r.URL.EscapedPath() != "/shop/private/a%20b.jpg" || r.URL.RawQuery != "X-Amz-Signature=abc" {
		t.Errorf("RustFS got %s?%s, want the key with its signature", r.URL.EscapedPath(), r.URL.RawQuery)
	}
	// a presigned URL's signature covers the host it was made for
	if r.Host != s.host {
		t.Errorf("RustFS got Host %q, want its own", r.Host)
	}
	if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
		t.Error("the visitor's cookies or credentials reached RustFS")
	}
	// an uploaded HTML or SVG file must not script the shared site
	if resp.Header.Get("Content-Security-Policy") != "sandbox" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("file headers = %v, want it sandboxed", resp.Header)
	}
}

func TestStartHostProxy_refusesAnythingButAFileOfTheBucket(t *testing.T) {
	rustfs, got := fakeRustFS(t)
	other, gotOther := fakeRustFS(t)
	s := storageAt(t, rustfs.URL, "shop")
	port := sharedProxy(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the site got %s %s", r.Method, r.URL.Path)
	}, s)

	files := "/__lerd_vite__/" + s.port
	for _, c := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/__lerd_vite__/" + storageAt(t, other.URL, "shop").port + "/shop/a.jpg", http.StatusNotFound},
		{"GET", "/__lerd_vite__/0" + s.port + "/shop/a.jpg", http.StatusNotFound},
		{"GET", files, http.StatusNotFound},
		{"GET", files + "/other/a.jpg", http.StatusNotFound},
		{"GET", files + "/shop/", http.StatusNotFound},
		{"GET", files + "/shop/public/", http.StatusNotFound},
		{"GET", files + "/shop/../other/a.jpg", http.StatusNotFound},
		{"GET", files + "/shop/%2e%2e/other/a.jpg", http.StatusNotFound},
		{"POST", files + "/shop/a.jpg", http.StatusMethodNotAllowed},
		{"PUT", files + "/shop/a.jpg", http.StatusMethodNotAllowed},
		{"DELETE", files + "/shop/a.jpg", http.StatusMethodNotAllowed},
	} {
		if resp, _ := tunnelRequest(t, port, c.method, c.path, nil); resp.StatusCode != c.status {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, resp.StatusCode, c.status)
		}
	}
	if len(*got)+len(*gotOther) != 0 {
		t.Errorf("loopback got %d requests, want none", len(*got)+len(*gotOther))
	}
}

func TestStartHostProxy_withoutLocalStorageThePrefixStaysClosed(t *testing.T) {
	rustfs, got := fakeRustFS(t)
	file := rustfs.URL + "/shop/a.jpg"
	var gotPath string
	port := sharedProxy(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<img src="%s">`, file)
	}, nil)

	path := "/__lerd_vite__/" + storageAt(t, rustfs.URL, "shop").port + "/shop/a.jpg"
	_, body := tunnelRequest(t, port, "GET", path, nil)
	// the prefix is a path of the site like any other, and nothing points at it
	if gotPath != path || len(*got) != 0 {
		t.Errorf("site got %q and loopback %d requests, want the path left to the site", gotPath, len(*got))
	}
	if !strings.Contains(body, file) {
		t.Errorf("body = %s, want the loopback URL left alone", body)
	}
}
