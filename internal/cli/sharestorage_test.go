package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func siteWithEnv(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeSiteEnv(t, dir, body)
	return dir
}

func TestSiteShareStorage_readsALocalBucket(t *testing.T) {
	for name, env := range map[string]string{
		"loopback":       "AWS_BUCKET=shop\nAWS_URL=http://localhost:9000/shop\n",
		"service domain": "AWS_BUCKET=shop\nAWS_URL=\"https://rustfs.test/shop\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := siteShareStorage(siteWithEnv(t, env))
			if s == nil || s.bucket != "shop" {
				t.Fatalf("storage = %+v, want the shop bucket", s)
			}
		})
	}
}

func TestSiteShareStorage_leavesOtherStorageAlone(t *testing.T) {
	for name, env := range map[string]string{
		"no s3":          "FILESYSTEM_DISK=local\n",
		"real bucket":    "AWS_BUCKET=shop\nAWS_URL=https://shop.s3.amazonaws.com\n",
		"cdn in front":   "AWS_BUCKET=shop\nAWS_URL=https://cdn.example.com/shop\n",
		"prefixed path":  "AWS_BUCKET=shop\nAWS_URL=http://localhost:9000/shop/media\n",
		"no bucket name": "AWS_URL=http://localhost:9000/shop\n",
	} {
		t.Run(name, func(t *testing.T) {
			if s := siteShareStorage(siteWithEnv(t, env)); s != nil {
				t.Errorf("storage = %+v, want none", s)
			}
		})
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
	return &shareStorage{bucket: bucket, origin: u}
}

func TestStartHostProxy_pointsBucketURLsAtTheTunnel(t *testing.T) {
	rustfs, _ := fakeRustFS(t)
	file := rustfs.URL + "/shop/public/cover.jpg"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Link", "<"+file+">; rel=preload; as=image")
		w.Header().Set("Location", file)
		fmt.Fprintf(w, `<img src="%s"><div data-page="{&quot;cover&quot;:&quot;%s&quot;}"></div><img src="%s/other/x.jpg">`,
			file, escapeSlashes(file), rustfs.URL)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false, storageAt(t, rustfs.URL, "shop"))
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	resp := doProxy(t, port, "abc.trycloudflare.com")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	shared := "https://abc.trycloudflare.com/__lerd_s3__/shop/public/cover.jpg"
	for _, want := range []string{`<img src="` + shared + `">`, escapeSlashes(shared), `<img src="` + rustfs.URL + `/other/x.jpg">`} {
		if !strings.Contains(string(body), want) {
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

func TestStartHostProxy_servesTheBucketsFiles(t *testing.T) {
	rustfs, got := fakeRustFS(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the site got %s, a bucket file is not its", r.URL.Path)
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false, storageAt(t, rustfs.URL, "shop"))
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/__lerd_s3__/shop/private/a%%20b.jpg?X-Amz-Signature=abc", port), nil)
	req.Host = "abc.trycloudflare.com"
	req.Header.Set("Cookie", "session=secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK || string(body) != "jpeg bytes" {
		t.Fatalf("got %d %q, want the file", resp.StatusCode, body)
	}
	r := (*got)[0]
	if r.URL.EscapedPath() != "/shop/private/a%20b.jpg" || r.URL.RawQuery != "X-Amz-Signature=abc" {
		t.Errorf("RustFS got %s?%s, want the key with its signature", r.URL.EscapedPath(), r.URL.RawQuery)
	}
	// a presigned URL's signature covers the host it was made for
	if r.Host != strings.TrimPrefix(rustfs.URL, "http://") {
		t.Errorf("RustFS got Host %q, want its own", r.Host)
	}
	if r.Header.Get("Cookie") != "" {
		t.Error("the visitor's cookies reached RustFS")
	}
	// an uploaded HTML or SVG file must not script the shared site
	if resp.Header.Get("Content-Security-Policy") != "sandbox" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("file headers = %v, want it sandboxed", resp.Header)
	}
}

func TestStartHostProxy_refusesAnythingButAFileOfTheBucket(t *testing.T) {
	rustfs, got := fakeRustFS(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false, storageAt(t, rustfs.URL, "shop"))
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	for _, c := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/__lerd_s3__/other/a.jpg", http.StatusNotFound},
		{"GET", "/__lerd_s3__/shop/", http.StatusNotFound},
		{"GET", "/__lerd_s3__/shop/public/", http.StatusNotFound},
		{"GET", "/__lerd_s3__/shop/../other/a.jpg", http.StatusNotFound},
		{"GET", "/__lerd_s3__/shop/%2e%2e/other/a.jpg", http.StatusNotFound},
		{"PUT", "/__lerd_s3__/shop/a.jpg", http.StatusMethodNotAllowed},
		{"DELETE", "/__lerd_s3__/shop/a.jpg", http.StatusMethodNotAllowed},
	} {
		req, _ := http.NewRequest(c.method, fmt.Sprintf("http://127.0.0.1:%d%s", port, c.path), nil)
		req.Host = "abc.trycloudflare.com"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.status {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, resp.StatusCode, c.status)
		}
	}
	if len(*got) != 0 {
		t.Errorf("RustFS got %d requests, want none", len(*got))
	}
}

func TestStartHostProxy_withoutABucketThePrefixIsTheSites(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer backend.Close()

	port, stop, err := startHostProxy("mysite.test", proxyPort(t, backend.URL), 0, false, nil)
	if err != nil {
		t.Fatalf("startHostProxy: %v", err)
	}
	defer stop()

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/__lerd_s3__/shop/a.jpg", port), nil)
	req.Host = "abc.trycloudflare.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/__lerd_s3__/shop/a.jpg" {
		t.Errorf("site got %q, want the path untouched", gotPath)
	}
}
