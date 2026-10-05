package ui

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/git"
)

// sourceContext is how many lines either side of the asked-for line a read
// returns, and sourceMaxLine how long a line may be before it is cut.
const (
	sourceContext = 8
	sourceMaxLine = 400
)

// sourceLine is one line of a file as the trace view shows it.
type sourceLine struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

// handleSource answers the lines around ?file=&line= for the Debug window's
// stack traces. Only a file inside a linked site or one of its worktrees is
// read, after its symlinks resolve, since the path comes from the page.
func handleSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !hasHostActionAuthority(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeSource(w, r)
}

// writeSource answers the lines around ?file=&line= once the caller has
// checked who asked.
func writeSource(w http.ResponseWriter, r *http.Request) {
	line, _ := strconv.Atoi(r.URL.Query().Get("line"))
	path, ok := siteSourcePath(r.URL.Query().Get("file"))
	if !ok || line < 1 {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	out := []sourceLine{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan() && n <= line+sourceContext; n++ {
		if n < line-sourceContext {
			continue
		}
		text := sc.Text()
		if len(text) > sourceMaxLine {
			text = text[:sourceMaxLine] + "…"
		}
		out = append(out, sourceLine{N: n, Text: text})
	}
	writeJSON(w, out)
}

// siteSourcePath resolves file to a regular file under a site's root or one
// of its worktrees, both with their symlinks resolved.
func siteSourcePath(file string) (string, bool) {
	if file == "" || !filepath.IsAbs(file) {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(file))
	if err != nil {
		return "", false
	}
	if st, err := os.Stat(real); err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	reg, err := config.LoadSites()
	if err != nil {
		return "", false
	}
	for _, s := range reg.Sites {
		roots := []string{s.Path}
		if wts, err := git.DetectWorktrees(s.Path, s.PrimaryDomain()); err == nil {
			for _, wt := range wts {
				roots = append(roots, wt.Path)
			}
		}
		for _, root := range roots {
			if root, err := filepath.EvalSymlinks(root); err == nil && strings.HasPrefix(real, root+string(os.PathSeparator)) {
				return real, true
			}
		}
	}
	return "", false
}
