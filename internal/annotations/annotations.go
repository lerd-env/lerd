// Package annotations keeps the notes a developer pins to elements of a site's
// pages from the debug bar, each with the element and the page view it was
// made on, until an assistant resolves it.
package annotations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
)

const (
	StatusOpen     = "open"
	StatusResolved = "resolved"
)

// Rect is where the element sat in the page, in CSS pixels from its top left.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Annotation is one note on one element of one page.
type Annotation struct {
	ID      string `json:"id"`
	Site    string `json:"site"`
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	RID     string `json:"rid,omitempty"`
	Comment string `json:"comment"`
	// The element: a selector that finds it again, its tag, the start of its
	// text, where it sat, and the component that rendered it where one says so.
	Selector   string `json:"selector"`
	Tag        string `json:"tag,omitempty"`
	Text       string `json:"text,omitempty"`
	Rect       *Rect  `json:"rect,omitempty"`
	Viewport   *Rect  `json:"viewport,omitempty"`
	Component  string `json:"component,omitempty"`
	Status     string `json:"status"`
	Created    string `json:"created"`
	Resolved   string `json:"resolved,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

var (
	mu   sync.Mutex
	safe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

func dir(site string) string { return filepath.Join(config.DataDir(), "annotations", site) }

func check(site, id string) error {
	if !safe.MatchString(site) || strings.Trim(site, ".") == "" {
		return fmt.Errorf("invalid site %q", site)
	}
	if id != "" && !safe.MatchString(id) {
		return fmt.Errorf("invalid annotation id %q", id)
	}
	return nil
}

func load(site string) ([]Annotation, error) {
	b, err := os.ReadFile(filepath.Join(dir(site), "annotations.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Annotation
	return list, json.Unmarshal(b, &list)
}

func save(site string, list []Annotation) error {
	if err := os.MkdirAll(dir(site), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir(site), "annotations.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir(site), "annotations.json"))
}

// Add keeps a new annotation.
func Add(a Annotation) (Annotation, error) {
	if err := check(a.Site, ""); err != nil {
		return a, err
	}
	if strings.TrimSpace(a.Comment) == "" || strings.TrimSpace(a.Selector) == "" {
		return a, errors.New("an annotation needs an element and a comment")
	}
	mu.Lock()
	defer mu.Unlock()
	list, err := load(a.Site)
	if err != nil {
		return a, err
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	a.ID = hex.EncodeToString(b)
	a.Status = StatusOpen
	a.Created = time.Now().UTC().Format(time.RFC3339)
	a.Resolved, a.Resolution = "", ""
	return a, save(a.Site, append(list, a))
}

// List returns a site's annotations, newest first, only those with the
// status when one is given.
func List(site, status string) ([]Annotation, error) {
	if err := check(site, ""); err != nil {
		return nil, err
	}
	mu.Lock()
	list, err := load(site)
	mu.Unlock()
	out := []Annotation{}
	for _, a := range list {
		if status == "" || a.Status == status {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out, err
}

// Get returns one annotation.
func Get(site, id string) (Annotation, error) {
	if err := check(site, id); err != nil {
		return Annotation{}, err
	}
	mu.Lock()
	defer mu.Unlock()
	list, err := load(site)
	if err != nil {
		return Annotation{}, err
	}
	for _, a := range list {
		if a.ID == id {
			return a, nil
		}
	}
	return Annotation{}, fmt.Errorf("no annotation %q on %s", id, site)
}

func change(site, id string, fn func(*Annotation)) (Annotation, error) {
	if err := check(site, id); err != nil {
		return Annotation{}, err
	}
	mu.Lock()
	defer mu.Unlock()
	list, err := load(site)
	if err != nil {
		return Annotation{}, err
	}
	for i := range list {
		if list[i].ID == id {
			fn(&list[i])
			return list[i], save(site, list)
		}
	}
	return Annotation{}, fmt.Errorf("no annotation %q on %s", id, site)
}

// Update rewrites an annotation's comment.
func Update(site, id, comment string) (Annotation, error) {
	if strings.TrimSpace(comment) == "" {
		return Annotation{}, errors.New("an annotation needs a comment")
	}
	return change(site, id, func(a *Annotation) { a.Comment = comment })
}

// Resolve marks an annotation dealt with, with what was done about it.
func Resolve(site, id, resolution string) (Annotation, error) {
	return change(site, id, func(a *Annotation) {
		a.Status = StatusResolved
		a.Resolution = resolution
		a.Resolved = time.Now().UTC().Format(time.RFC3339)
	})
}

// Reopen puts a resolved annotation back on the page.
func Reopen(site, id string) (Annotation, error) {
	return change(site, id, func(a *Annotation) { a.Status, a.Resolved, a.Resolution = StatusOpen, "", "" })
}

// Delete forgets an annotation.
func Delete(site, id string) error {
	if err := check(site, id); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	list, err := load(site)
	if err != nil {
		return err
	}
	for i, a := range list {
		if a.ID == id {
			return save(site, append(list[:i], list[i+1:]...))
		}
	}
	return fmt.Errorf("no annotation %q on %s", id, site)
}
