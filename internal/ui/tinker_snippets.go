package ui

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// tinkerSnippet is one reusable snippet the Tinker tab's picker can load into
// the editor. They are plain .php files: committed in the project (shared with
// the team) or in the user's config dir (personal, follow every site).
type tinkerSnippet struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Source  string `json:"source"`
	Content string `json:"content"`
}

// maxSnippetSize mirrors the tinker endpoint's request cap; a larger file
// could never run, so it is skipped rather than truncated.
const maxSnippetSize = 64 << 10

var snippetNameRe = regexp.MustCompile(`^\s*//\s*@name(?::\s*|\s+)(.+?)\s*$`)

// listTinkerSnippets gathers snippets from the project's .lerd/tinker/snippets,
// its .tinkerwell/snippets (so Tinkerwell users keep theirs for free), and the
// global ~/.config/lerd/tinker/snippets, in that order.
func listTinkerSnippets(siteDir string) []tinkerSnippet {
	out := []tinkerSnippet{}
	out = append(out, readSnippetDir(filepath.Join(siteDir, ".lerd", "tinker", "snippets"), "project")...)
	out = append(out, readSnippetDir(filepath.Join(siteDir, ".tinkerwell", "snippets"), "tinkerwell")...)
	out = append(out, readSnippetDir(filepath.Join(config.ConfigDir(), "tinker", "snippets"), "global")...)
	return out
}

func readSnippetDir(dir, source string) []tinkerSnippet {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []tinkerSnippet
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".php") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > maxSnippetSize {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		out = append(out, tinkerSnippet{
			Name:    name,
			Label:   snippetLabel(name, string(content)),
			Source:  source,
			Content: string(content),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Label), strings.ToLower(out[j].Label)
		if a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// snippetLabel prefers a "// @name …" header in the first lines and falls
// back to the filename without its extension.
func snippetLabel(name, content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 10 {
		lines = lines[:10]
	}
	for _, l := range lines {
		if m := snippetNameRe.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return strings.TrimSuffix(name, ".php")
}

// snippetWriteDir maps a source to the directory writes may touch. The
// tinkerwell dir is read-only: lerd never manages another tool's files.
func snippetWriteDir(siteDir, source string) string {
	switch source {
	case "project":
		return filepath.Join(siteDir, ".lerd", "tinker", "snippets")
	case "global":
		return filepath.Join(config.ConfigDir(), "tinker", "snippets")
	}
	return ""
}

// validSnippetName accepts a bare filename: no separators (closes traversal
// out of the snippet dir) and no leading dot (dotfiles are never listed).
func validSnippetName(name string) bool {
	return name != "" && len(name) <= 100 &&
		!strings.HasPrefix(name, ".") && !strings.ContainsAny(name, "/\\")
}

func handleSiteTinkerSnippets(w http.ResponseWriter, r *http.Request, domain string) {
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	dir := resolveSitePath(site, r.URL.Query().Get("branch"))
	if dir == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, listTinkerSnippets(dir))
	case http.MethodPost:
		saveSiteTinkerSnippet(w, r, dir)
	case http.MethodDelete:
		deleteSiteTinkerSnippet(w, r, dir)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func saveSiteTinkerSnippet(w http.ResponseWriter, r *http.Request, siteDir string) {
	var body struct {
		Name    string `json:"name"`
		Source  string `json:"source"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSnippetSize+4096)).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if !strings.HasSuffix(name, ".php") {
		name += ".php"
	}
	if !validSnippetName(name) {
		http.Error(w, "invalid snippet name", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		http.Error(w, "snippet is empty", http.StatusBadRequest)
		return
	}
	// Mirror the listing's skip rule: a file over the cap would be written
	// and then never shown, which reads as a silently lost snippet.
	if len(body.Content) > maxSnippetSize {
		http.Error(w, "snippet is larger than 64 KB", http.StatusBadRequest)
		return
	}
	dir := snippetWriteDir(siteDir, body.Source)
	if dir == "" {
		http.Error(w, "snippets can only be saved to the project or the global folder", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "creating snippet dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body.Content), 0o644); err != nil {
		http.Error(w, "writing snippet: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, listTinkerSnippets(siteDir))
}

func deleteSiteTinkerSnippet(w http.ResponseWriter, r *http.Request, siteDir string) {
	name := r.URL.Query().Get("name")
	if !validSnippetName(name) || !strings.HasSuffix(name, ".php") {
		http.Error(w, "invalid snippet name", http.StatusBadRequest)
		return
	}
	dir := snippetWriteDir(siteDir, r.URL.Query().Get("source"))
	if dir == "" {
		http.Error(w, "snippets can only be deleted from the project or the global folder", http.StatusBadRequest)
		return
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "deleting snippet: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, listTinkerSnippets(siteDir))
}
