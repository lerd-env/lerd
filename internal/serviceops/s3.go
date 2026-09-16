package serviceops

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/geodro/lerd/internal/config"
)

// s3Driver is what an entity's driver field carries when lerd talks to the
// service's S3 API itself instead of running a client image. The images that
// used to do this are public ones lerd does not control: Docker Hub stopped
// serving minio/mc mid-flight and a fresh install could not create a bucket at
// all, so the client is compiled in and there is nothing left to pull.
const s3Driver = "s3"

// s3Service is the service lerd provisions site buckets on. The .env wiring
// that asks for a bucket already names rustfs, so the provisioner reads its
// declared connection rather than carrying a second copy of it.
const s3Service = "rustfs"

// s3Timeout caps one S3 call chain. Listing a bucket walks every object, so
// this is generous while still unsticking a service that stopped answering.
const s3Timeout = 2 * time.Minute

// mirrorTimeout caps a whole bucket mirror: an import walks every object of a
// bucket that may hold years of uploads.
const mirrorTimeout = 2 * time.Hour

// s3Settings is the connection an entity declares in its env: the port the S3
// API listens on inside the container, and the credentials lerd provisions the
// service with.
type s3Settings struct {
	port   int
	access string
	secret string
}

// parseS3Env reads the S3_PORT, S3_ACCESS_KEY and S3_SECRET_KEY pairs a driven
// entity declares. Fails at the declaration rather than at the first request,
// so a preset that forgot one says so.
func parseS3Env(env []string) (s3Settings, error) {
	var s s3Settings
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "S3_PORT":
			s.port, _ = strconv.Atoi(strings.TrimSpace(value))
		case "S3_ACCESS_KEY":
			s.access = strings.TrimSpace(value)
		case "S3_SECRET_KEY":
			s.secret = strings.TrimSpace(value)
		}
	}
	if s.port <= 0 || s.access == "" || s.secret == "" {
		return s3Settings{}, fmt.Errorf("s3 entity needs S3_PORT, S3_ACCESS_KEY and S3_SECRET_KEY in its env")
	}
	return s, nil
}

// s3Endpoint is where the service answers from the host: the published port,
// which is not the container port once the user has moved the service off it.
func s3Endpoint(service string, s s3Settings) string {
	return fmt.Sprintf("127.0.0.1:%d", config.ServiceConfigFor(service).HostPortFor(s.port, s.port, true))
}

// s3ClientFor builds the client for a driven entity of the named service.
func s3ClientFor(service string, env []string) (*minio.Client, error) {
	s, err := parseS3Env(env)
	if err != nil {
		return nil, err
	}
	return newS3Client(s3Endpoint(service, s), s.access, s.secret)
}

func newS3Client(endpoint, access, secret string) (*minio.Client, error) {
	return minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(access, secret, ""),
		Secure:       false,
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	})
}

// s3ListBuckets returns one row per bucket with its object count and total
// size, the columns the buckets panel declares.
func s3ListBuckets(ctx context.Context, c *minio.Client) ([]EntityRow, error) {
	buckets, err := c.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]EntityRow, 0, len(buckets))
	for _, b := range buckets {
		var objects, size int64
		for obj := range c.ListObjects(ctx, b.Name, minio.ListObjectsOptions{Recursive: true}) {
			if obj.Err != nil {
				return nil, obj.Err
			}
			objects++
			size += obj.Size
		}
		rows = append(rows, EntityRow{
			Name:   b.Name,
			Values: []string{strconv.FormatInt(objects, 10), strconv.FormatInt(size, 10)},
		})
	}
	return rows, nil
}

// s3PublicPolicy is the anonymous read-write policy lerd gives every bucket it
// creates, so a local app can serve an upload straight from its URL without
// signing it. It is the policy `mc anonymous set public` used to write.
func s3PublicPolicy(bucket string) string {
	return `{"Version":"2012-10-17","Statement":[` +
		`{"Effect":"Allow","Principal":{"AWS":["*"]},` +
		`"Action":["s3:GetBucketLocation","s3:ListBucket","s3:ListBucketMultipartUploads"],` +
		`"Resource":["arn:aws:s3:::` + bucket + `"]},` +
		`{"Effect":"Allow","Principal":{"AWS":["*"]},` +
		`"Action":["s3:AbortMultipartUpload","s3:DeleteObject","s3:GetObject","s3:ListMultipartUploadParts","s3:PutObject"],` +
		`"Resource":["arn:aws:s3:::` + bucket + `/*"]}]}`
}

// s3CreateBucket creates the bucket and opens it to anonymous clients.
func s3CreateBucket(ctx context.Context, c *minio.Client, name string) error {
	if err := c.MakeBucket(ctx, name, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		return err
	}
	return c.SetBucketPolicy(ctx, name, s3PublicPolicy(name))
}

// s3RemoveBucket empties the bucket and removes it, the force a bucket drop
// from the UI has always had.
func s3RemoveBucket(ctx context.Context, c *minio.Client, name string) error {
	var listErr error
	objects := make(chan minio.ObjectInfo)
	go func() {
		defer close(objects)
		for obj := range c.ListObjects(ctx, name, minio.ListObjectsOptions{Recursive: true}) {
			if obj.Err != nil {
				listErr = obj.Err
				return
			}
			objects <- obj
		}
	}()
	for res := range c.RemoveObjects(ctx, name, objects, minio.RemoveObjectsOptions{}) {
		if res.Err != nil {
			return fmt.Errorf("emptying bucket %q: %w", name, res.Err)
		}
	}
	if listErr != nil {
		return fmt.Errorf("listing bucket %q: %w", name, listErr)
	}
	return c.RemoveBucket(ctx, name)
}

// listS3Entities serves a driven entity's list straight from the service's S3
// API.
func listS3Entities(service string, spec *config.EntitySpec) ([]EntityRow, error) {
	c, err := s3ClientFor(service, spec.Env)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3Timeout)
	defer cancel()
	return s3ListBuckets(ctx, c)
}

// runS3EntityAction serves a driven entity's declared action. The name is
// validated the same way a name spliced into a shell command is, so a bad one
// is refused here rather than by the server.
func runS3EntityAction(service string, spec *config.EntitySpec, action, name string) error {
	if err := ValidateDatabaseName(name); err != nil {
		return err
	}
	c, err := s3ClientFor(service, spec.Env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3Timeout)
	defer cancel()
	switch action {
	case "create":
		return s3CreateBucket(ctx, c, name)
	case "delete":
		return s3RemoveBucket(ctx, c, name)
	}
	return fmt.Errorf("%s does not support %s on %s", service, action, spec.Kind)
}

// S3Remote is an S3 endpoint outside lerd, the shape an import reads from.
type S3Remote struct {
	Endpoint string
	Access   string
	Secret   string
}

func (r S3Remote) client() (*minio.Client, error) {
	return newS3Client(r.Endpoint, r.Access, r.Secret)
}

// S3RemoteBuckets lists the buckets a remote holds.
func S3RemoteBuckets(r S3Remote) ([]string, error) {
	c, err := r.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3Timeout)
	defer cancel()
	buckets, err := c.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(buckets))
	for _, b := range buckets {
		names = append(names, b.Name)
	}
	return names, nil
}

// S3RemoteObjectCount counts the objects in one bucket of a remote.
func S3RemoteObjectCount(r S3Remote, bucket string) (int, error) {
	c, err := r.client()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3Timeout)
	defer cancel()
	count := 0
	for obj := range c.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return 0, obj.Err
		}
		count++
	}
	return count, nil
}

// S3MirrorInto copies every object of a remote bucket into a bucket on lerd's
// own S3 service, overwriting what is already there under the same key.
func S3MirrorInto(r S3Remote, sourceBucket, destBucket string, progress func(copied int)) error {
	src, err := r.client()
	if err != nil {
		return err
	}
	spec := EntityFor(s3Service, "buckets")
	if spec == nil || spec.Driver != s3Driver {
		return fmt.Errorf("%s declares no S3 buckets", s3Service)
	}
	dst, err := s3ClientFor(s3Service, spec.Env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
	defer cancel()

	copied := 0
	for obj := range src.ListObjects(ctx, sourceBucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return obj.Err
		}
		body, err := src.GetObject(ctx, sourceBucket, obj.Key, minio.GetObjectOptions{})
		if err != nil {
			return fmt.Errorf("reading %q: %w", obj.Key, err)
		}
		_, err = dst.PutObject(ctx, destBucket, obj.Key, body, obj.Size,
			minio.PutObjectOptions{ContentType: obj.ContentType})
		body.Close()
		if err != nil {
			return fmt.Errorf("writing %q: %w", obj.Key, err)
		}
		copied++
		if progress != nil {
			progress(copied)
		}
	}
	return nil
}
