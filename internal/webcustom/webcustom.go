// Package webcustom reads the per-site and global customisation lerd's own
// web server applies, the declarative counterpart of nginx's custom.d.
package webcustom

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/geodro/lerd/internal/config"
)

// Dir holds the files: global.yaml and custom.d/<domain>.yaml.
func Dir() string { return filepath.Join(config.DataDir(), "web") }

func GlobalPath() string { return filepath.Join(Dir(), "global.yaml") }

// SitePath is the file for a site's own domain or a worktree's subdomain.
func SitePath(domain string) string { return filepath.Join(Dir(), "custom.d", domain+".yaml") }

// Custom is what applies to one domain, the global file merged with its own.
type Custom struct {
	// Headers are set on every response.
	Headers map[string]string
	// Redirects answer a path, or anything under it, with Status to To.
	Redirects []Redirect
	// Proxies send a path, and anything under it, to another server.
	Proxies []Proxy
	// Roots serve a path, and anything under it, from another directory,
	// PHP included, as Magento's /setup is served from the project root.
	Roots []Root
	// MaxBodyBytes caps a request body, 0 for no cap, when MaxBodySet.
	MaxBodyBytes int64
	MaxBodySet   bool
	// FastCGIParams are added to every PHP request.
	FastCGIParams map[string]string
}

type Redirect struct {
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Status int    `yaml:"status"`
}

type Proxy struct {
	Path string `yaml:"path"`
	To   string `yaml:"to"`
}

type Root struct {
	Path string `yaml:"path"`
	Root string `yaml:"root"`
}

type file struct {
	Headers       map[string]string `yaml:"headers"`
	Redirects     []Redirect        `yaml:"redirects"`
	Proxies       []Proxy           `yaml:"proxies"`
	Roots         []Root            `yaml:"roots"`
	MaxBodySize   *string           `yaml:"max_body_size"`
	FastCGIParams map[string]string `yaml:"fastcgi_params"`
}

// For reads the customisation for domain: the global file, then the site's
// own, whose headers and params win and whose rules come first. A file that
// cannot be served is an error naming it, never a silent no-op.
func For(domain string) (Custom, error) {
	c := Custom{Headers: map[string]string{}, FastCGIParams: map[string]string{}}
	for _, path := range []string{GlobalPath(), SitePath(domain)} {
		f, err := load(path)
		if err != nil {
			return Custom{}, err
		}
		if f == nil {
			continue
		}
		maps.Copy(c.Headers, f.Headers)
		maps.Copy(c.FastCGIParams, f.FastCGIParams)
		c.Redirects = append(f.Redirects, c.Redirects...)
		c.Proxies = append(f.Proxies, c.Proxies...)
		c.Roots = append(f.Roots, c.Roots...)
		if f.MaxBodySize != nil {
			c.MaxBodyBytes, _ = parseSize(*f.MaxBodySize)
			c.MaxBodySet = true
		}
	}
	return c, nil
}

// Validate checks a file's content as For would read it, for an editor to
// refuse a save before it reaches the server.
func Validate(name string, data []byte) error {
	_, err := parse(name, data)
	return err
}

func load(path string) (*file, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parse(path, data)
}

var (
	headerName = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")
	paramName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func parse(path string, data []byte) (*file, error) {
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%s: %s", path, fmt.Sprintf(format, a...))
	}
	for k, v := range f.Headers {
		if !headerName.MatchString(k) {
			return nil, bad("header %q is not a valid header name", k)
		}
		if strings.ContainsAny(v, "\r\n\x00") {
			return nil, bad("header %s value carries a line break", k)
		}
	}
	for _, r := range f.Redirects {
		if !strings.HasPrefix(r.From, "/") || r.To == "" {
			return nil, bad("redirect needs a from starting with / and a to, got from %q to %q", r.From, r.To)
		}
		switch r.Status {
		case 0, 301, 302, 307, 308:
		default:
			return nil, bad("redirect %s status %d is not 301, 302, 307 or 308", r.From, r.Status)
		}
	}
	for _, p := range f.Proxies {
		u, err := url.Parse(p.To)
		if !strings.HasPrefix(p.Path, "/") || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, bad("proxy needs a path starting with / and an http(s) to, got path %q to %q", p.Path, p.To)
		}
	}
	for _, r := range f.Roots {
		if !strings.HasPrefix(r.Path, "/") || r.Root == "" || strings.Contains(ExpandRoot(r.Root, "", ""), "{{") {
			return nil, bad("root needs a path starting with / and a root using only {{root}} and {{public}}, got path %q root %q", r.Path, r.Root)
		}
	}
	if f.MaxBodySize != nil {
		if _, err := parseSize(*f.MaxBodySize); err != nil {
			return nil, bad("max_body_size %q: %v", *f.MaxBodySize, err)
		}
	}
	for k, v := range f.FastCGIParams {
		if !paramName.MatchString(k) || strings.ContainsAny(v, "\r\n\x00") {
			return nil, bad("fastcgi param %q is not a valid name or its value carries a line break", k)
		}
	}
	return &f, nil
}

// parseSize reads a size the way nginx's client_max_body_size does: bytes, or
// a k, m or g suffix, and 0 for no cap.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1<<10, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1<<20, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "g"):
		mult, s = 1<<30, strings.TrimSuffix(s, "g")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("not a size like 0, 512k, 100m or 1g")
	}
	return n * mult, nil
}

// ExpandRoot fills {{root}} (the project) and {{public}} (its document root).
func ExpandRoot(v, root, public string) string {
	return strings.NewReplacer("{{root}}", root, "{{public}}", public).Replace(v)
}
