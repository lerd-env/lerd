package serviceops

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fakeS3 is enough of the S3 API for the bucket panel: it answers the calls
// lerd makes and records the bodies it was sent.
type fakeS3 struct {
	buckets map[string][]int64 // bucket -> object sizes
	policy  string
	created []string
	deleted []string
	purged  int
}

func newFakeS3(t *testing.T, f *fakeS3) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bucket := strings.Trim(r.URL.Path, "/")
		q := r.URL.Query()
		switch {
		case r.Method == http.MethodGet && bucket == "":
			var b strings.Builder
			b.WriteString("<ListAllMyBucketsResult><Owner><ID>lerd</ID></Owner><Buckets>")
			for name := range f.buckets {
				fmt.Fprintf(&b, "<Bucket><Name>%s</Name><CreationDate>2026-01-01T00:00:00.000Z</CreationDate></Bucket>", name)
			}
			b.WriteString("</Buckets></ListAllMyBucketsResult>")
			io.WriteString(w, b.String()) //nolint:errcheck
		case r.Method == http.MethodGet && q.Get("list-type") == "2":
			var b strings.Builder
			fmt.Fprintf(&b, "<ListBucketResult><Name>%s</Name><IsTruncated>false</IsTruncated>", bucket)
			for i, size := range f.buckets[bucket] {
				fmt.Fprintf(&b, "<Contents><Key>file-%d</Key><Size>%d</Size>"+
					"<LastModified>2026-01-01T00:00:00.000Z</LastModified><ETag>\"x\"</ETag></Contents>", i, size)
			}
			b.WriteString("</ListBucketResult>")
			io.WriteString(w, b.String()) //nolint:errcheck
		case r.Method == http.MethodPut && q.Has("policy"):
			body, _ := io.ReadAll(r.Body)
			f.policy = string(body)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut:
			f.created = append(f.created, bucket)
			f.buckets[bucket] = nil
		case r.Method == http.MethodPost && q.Has("delete"):
			body, _ := io.ReadAll(r.Body)
			f.purged += strings.Count(string(body), "<Object>")
			io.WriteString(w, "<DeleteResult></DeleteResult>") //nolint:errcheck
		case r.Method == http.MethodDelete:
			f.deleted = append(f.deleted, bucket)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodHead:
			if _, ok := f.buckets[bucket]; !ok {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeS3Env is the env a driven entity declares, pointed at the stub.
func fakeS3Env(t *testing.T, srv *httptest.Server) []string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return []string{"S3_PORT=" + u.Port(), "S3_ACCESS_KEY=lerd", "S3_SECRET_KEY=lerdpassword"}
}

// A preset that forgot a connection field must say so at the declaration, not
// fail with an unsigned request against a default endpoint.
func TestParseS3EnvRequiresEveryField(t *testing.T) {
	s, err := parseS3Env([]string{"S3_PORT=9000", "S3_ACCESS_KEY=lerd", "S3_SECRET_KEY=lerdpassword"})
	if err != nil {
		t.Fatalf("complete env: %v", err)
	}
	if s.port != 9000 || s.access != "lerd" || s.secret != "lerdpassword" {
		t.Errorf("parsed = %+v", s)
	}
	for _, env := range [][]string{
		{"S3_ACCESS_KEY=lerd", "S3_SECRET_KEY=x"},
		{"S3_PORT=9000", "S3_SECRET_KEY=x"},
		{"S3_PORT=9000", "S3_ACCESS_KEY=lerd"},
		{"S3_PORT=nope", "S3_ACCESS_KEY=lerd", "S3_SECRET_KEY=x"},
	} {
		if _, err := parseS3Env(env); err == nil {
			t.Errorf("parseS3Env(%v) = nil error, want one", env)
		}
	}
}

// The panel must reach the service where it actually listens: a user who moved
// rustfs off 9000 published the override, not the container port.
func TestS3EndpointFollowsThePublishedPort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Services = map[string]config.ServiceConfig{"rustfs": {Image: "rustfs:1", Port: 9000, PublishedPort: 9100}}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if got := s3Endpoint("rustfs", s3Settings{port: 9000}); got != "127.0.0.1:9100" {
		t.Errorf("s3Endpoint = %q, want the published port", got)
	}
	if got := s3Endpoint("nosuchservice", s3Settings{port: 9000}); got != "127.0.0.1:9000" {
		t.Errorf("s3Endpoint = %q, want the declared port", got)
	}
}

func TestS3ListBucketsReportsObjectsAndSize(t *testing.T) {
	f := &fakeS3{buckets: map[string][]int64{"acme": {100, 200}, "empty": nil}}
	srv := newFakeS3(t, f)
	c, err := s3ClientFor("rustfs", fakeS3Env(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s3ListBuckets(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, r := range rows {
		got[r.Name] = r.Values
	}
	if len(got) != 2 {
		t.Fatalf("rows = %v", got)
	}
	if v := got["acme"]; len(v) != 2 || v[0] != "2" || v[1] != "300" {
		t.Errorf("acme = %v, want 2 objects totalling 300 bytes", v)
	}
	if v := got["empty"]; len(v) != 2 || v[0] != "0" || v[1] != "0" {
		t.Errorf("empty = %v, want zeroes", v)
	}
}

// A site serves its uploads straight off the bucket URL, so a created bucket
// has to be open to anonymous clients the way mc's "public" policy left it.
func TestS3CreateBucketOpensItToAnonymousClients(t *testing.T) {
	f := &fakeS3{buckets: map[string][]int64{}}
	srv := newFakeS3(t, f)
	c, err := s3ClientFor("rustfs", fakeS3Env(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if err := s3CreateBucket(context.Background(), c, "acme"); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 || f.created[0] != "acme" {
		t.Errorf("created = %v", f.created)
	}
	for _, want := range []string{`"AWS":["*"]`, "s3:GetObject", "s3:PutObject", "arn:aws:s3:::acme/*"} {
		if !strings.Contains(f.policy, want) {
			t.Errorf("policy %s is missing %q", f.policy, want)
		}
	}
}

func TestS3RemoveBucketEmptiesItFirst(t *testing.T) {
	f := &fakeS3{buckets: map[string][]int64{"acme": {1, 2, 3}}}
	srv := newFakeS3(t, f)
	c, err := s3ClientFor("rustfs", fakeS3Env(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	if err := s3RemoveBucket(context.Background(), c, "acme"); err != nil {
		t.Fatal(err)
	}
	if f.purged != 3 {
		t.Errorf("purged %d objects, want the 3 the bucket held", f.purged)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "acme" {
		t.Errorf("deleted = %v", f.deleted)
	}
}

// The engine must serve a driven entity itself: no client image, no shell.
func TestListEntitiesServesTheS3Driver(t *testing.T) {
	f := &fakeS3{buckets: map[string][]int64{"acme": {100}}}
	srv := newFakeS3(t, f)
	u, _ := url.Parse(srv.URL)
	writeCustomService(t, "objectstore", `name: objectstore
image: example/store:1
introspect:
  entities:
    - kind: buckets
      driver: s3
      env:
        - S3_PORT=`+u.Port()+`
        - S3_ACCESS_KEY=lerd
        - S3_SECRET_KEY=lerdpassword
      actions:
        create:
        delete:
          destructive: true
`)
	spec := EntityFor("objectstore", "buckets")
	if spec == nil {
		t.Fatal("driven entity did not resolve")
	}
	rows, err := ListEntities("objectstore", spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "acme" {
		t.Fatalf("rows = %+v", rows)
	}
	if err := RunEntityAction("objectstore", spec, "create", "other"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(f.created) != 1 || f.created[0] != "other" {
		t.Errorf("created = %v", f.created)
	}
	if err := RunEntityAction("objectstore", spec, "grow", "other"); err == nil {
		t.Error("an undeclared action must fail")
	}
}

// The provisioner reads the connection off the rustfs preset, so a site that
// links to a moved rustfs still gets its bucket.
func TestEnsureS3BucketCreatesOnceAndReportsExisting(t *testing.T) {
	f := &fakeS3{buckets: map[string][]int64{}}
	srv := newFakeS3(t, f)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Services = map[string]config.ServiceConfig{"rustfs": {Image: "rustfs:1", Port: 9000, PublishedPort: port}}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureS3Bucket("acme")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("first call should report the bucket as created")
	}
	created, err = EnsureS3Bucket("acme")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second call should report the bucket as already there")
	}
	if len(f.created) != 1 {
		t.Errorf("created = %v, want one bucket", f.created)
	}
	if _, err := EnsureS3Bucket("../escape"); err == nil {
		t.Error("an invalid bucket name must be refused")
	}
}
