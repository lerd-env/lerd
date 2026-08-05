package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// An IDE runs its quality tools against a copy of the buffer you are editing
// rather than against the file in the project tree: PhpStorm writes that copy
// into its own temp directory and hands the path to phpstan, php-cs-fixer,
// phpcbf or psalm. The container never sees that path, so the tool reports a
// file that does not exist. The script-operand rescue in php_exec.go cannot
// help here, since /dev/stdin is not a path a tool can stat, reopen or write
// back to.
//
// So every unreachable path argument is staged into a scratch directory under
// the lerd data dir, which the container already reaches at an identical path,
// and the argument is rewritten to point at the copy. Output is mapped back so
// diagnostics still name the path the caller passed, and a copy the tool edited
// is written home again so the in-place fixers keep working.

// stageSizeLimit caps what staging will copy. An IDE scratch file is a few
// kilobytes; a path this large is a real data file whose owner wants a mount,
// not a duplicate.
const stageSizeLimit = 64 << 20

// stagedPath is one argument's host path and the copy the container reads.
type stagedPath struct {
	orig   string
	staged string
	dir    bool
}

// argStage owns the scratch tree for one exec, so it can be synced back and
// removed once the child has exited.
type argStage struct {
	root  string
	items []stagedPath
}

// splitPathArg picks the absolute host path out of an argument, returning the
// option prefix that has to be put back in front of the rewritten value. Only
// two shapes carry one: a bare token, and a long option's glued value. A short
// option's value is a bare token in its own right, so it is covered by the
// first; php's own `-d key=path` is not, which is why a single dash never
// yields a path here.
func splitPathArg(arg string) (prefix, path string, ok bool) {
	if strings.HasPrefix(arg, "--") {
		name, value, found := strings.Cut(arg, "=")
		if !found || !filepath.IsAbs(value) {
			return "", "", false
		}
		return name + "=", value, true
	}
	if strings.HasPrefix(arg, "-") || !filepath.IsAbs(arg) {
		return "", "", false
	}
	return "", arg, true
}

// stagePathArgIndexes returns the indexes of the arguments naming a host path
// the container cannot reach. Only the tool's own arguments are eligible:
// everything up to and including the script operand belongs to the php
// interpreter, whose option values are not paths to stage, and the operand
// itself is already answered by the /dev/stdin rescue.
func stagePathArgIndexes(args []string, scriptIdx int, needsStaging func(string) bool) []int {
	if scriptIdx < 0 {
		return nil
	}
	var out []int
	for i := scriptIdx + 1; i < len(args); i++ {
		_, path, ok := splitPathArg(args[i])
		if ok && needsStaging(path) {
			out = append(out, i)
		}
	}
	return out
}

// unreachableInContainer reports whether a path exists on the host but cannot
// be read from the PHP container for the given version.
func unreachableInContainer(path, version string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return !podman.PathVisible(path, version)
}

// stageRoot returns the directory scratch trees are created under, and whether
// the container can actually see it. An XDG data dir outside home is reachable
// only if it happens to be mounted, and staging into a path the container
// cannot read would trade one invisible file for another.
func stageRoot(version string) (string, bool) {
	root := filepath.Join(config.DataDir(), "shim-stage")
	if !podman.PathVisible(root, version) {
		return "", false
	}
	return root, true
}

// stageArgs copies each listed argument's path into a fresh scratch tree under
// root and returns the argument list rewritten to point at the copies. Each
// path gets its own numbered directory so two arguments sharing a basename do
// not collide, and the basename itself is kept because tools key off the
// extension and report the name.
func stageArgs(args []string, idxs []int, root string) (*argStage, []string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, nil, err
	}
	dir, err := os.MkdirTemp(root, "run-")
	if err != nil {
		return nil, nil, err
	}
	stage := &argStage{root: dir}

	rewritten := append([]string(nil), args...)
	for n, i := range idxs {
		prefix, path, ok := splitPathArg(args[i])
		if !ok {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			stage.remove()
			return nil, nil, err
		}
		if err := checkStageSize(path, info); err != nil {
			stage.remove()
			return nil, nil, err
		}
		slot := filepath.Join(dir, fmt.Sprint(n))
		if err := os.MkdirAll(slot, 0o755); err != nil {
			stage.remove()
			return nil, nil, err
		}
		staged := filepath.Join(slot, filepath.Base(path))
		if err := copyStagePath(path, staged, info); err != nil {
			stage.remove()
			return nil, nil, err
		}
		stage.items = append(stage.items, stagedPath{orig: path, staged: staged, dir: info.IsDir()})
		rewritten[i] = prefix + staged
	}
	return stage, rewritten, nil
}

// checkStageSize refuses a path too large to copy, naming both ways of making
// it reachable for real instead.
func checkStageSize(path string, info fs.FileInfo) error {
	var total int64
	if info.IsDir() {
		err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			total += fi.Size()
			if total > stageSizeLimit {
				return fs.SkipAll
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		total = info.Size()
	}
	if total > stageSizeLimit {
		return fmt.Errorf("%s is too large for lerd to copy into the PHP container. Park the directory it lives in, or add it to mounts: in %s", path, config.GlobalConfigFile())
	}
	return nil
}

func copyStagePath(src, dst string, info fs.FileInfo) error {
	if info.IsDir() {
		return copyStageTree(src, dst)
	}
	return copyStageFile(src, dst, info.Mode().Perm())
}

func copyStageFile(src, dst string, mode fs.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func copyStageTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil // sockets and devices have nothing to analyse
		}
		return copyStageFile(path, target, info.Mode().Perm())
	})
}

// syncBack copies a staged path the tool rewrote over the caller's own path, so
// an in-place fixer's work survives the round trip. Content is compared rather
// than timestamps: a tool that read and rewrote a file unchanged should not
// touch the original's mtime and set the IDE reloading it.
func (s *argStage) syncBack() error {
	if s == nil {
		return nil
	}
	for _, item := range s.items {
		if !item.dir {
			if err := syncBackFile(item.staged, item.orig); err != nil {
				return err
			}
			continue
		}
		err := filepath.WalkDir(item.staged, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(item.staged, path)
			if err != nil {
				return err
			}
			return syncBackFile(path, filepath.Join(item.orig, rel))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func syncBackFile(staged, orig string) error {
	got, err := os.ReadFile(staged)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // the tool deleted its copy; leave the caller's file alone
		}
		return err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(orig); err == nil {
		if had, err := os.ReadFile(orig); err == nil && bytes.Equal(had, got) {
			return nil
		}
		mode = info.Mode().Perm()
	}
	return os.WriteFile(orig, got, mode)
}

// finish flushes the mapped output streams and writes any edited copy home. It
// is the tail of every staged run, whether the tool succeeded or not: a fixer
// exits non-zero precisely because it changed something, and those changes
// still have to be saved.
func (s *argStage) finish(writers ...*pathMapWriter) error {
	for _, w := range writers {
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return s.syncBack()
}

func (s *argStage) remove() {
	if s == nil || s.root == "" {
		return
	}
	os.RemoveAll(s.root)
}

// pathRep is one substitution the output rewriter applies.
type pathRep struct{ from, to []byte }

// pathMapWriter rewrites staged paths back to the ones the caller passed as the
// tool's output streams through it. An IDE keys the diagnostics it renders to
// the path it handed in, so the substitution has to survive a chunk boundary
// falling inside a path: the longest form of a staged path, less one byte, is
// held back on every write and flushed at the end. Only the path text is
// mapped, so a tool that also prints the length of one (var_dump of a string
// holding it) reports the staged length beside the original path.
type pathMapWriter struct {
	dst  io.Writer
	reps []pathRep
	buf  []byte
	hold int
}

func newPathMapWriter(dst io.Writer, items []stagedPath) *pathMapWriter {
	w := &pathMapWriter{dst: dst}
	for _, item := range items {
		// The JSON error formats an IDE asks these tools for run through
		// json_encode, which escapes every forward slash, so the same path has to
		// be matched in both shapes and put back in the shape it was found in.
		w.reps = append(w.reps,
			pathRep{from: []byte(item.staged), to: []byte(item.orig)},
			pathRep{from: []byte(jsonEscapeSlashes(item.staged)), to: []byte(jsonEscapeSlashes(item.orig))},
		)
	}
	for _, rep := range w.reps {
		if n := len(rep.from) - 1; n > w.hold {
			w.hold = n
		}
	}
	return w
}

func jsonEscapeSlashes(path string) string {
	return strings.ReplaceAll(path, "/", `\/`)
}

func (w *pathMapWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.buf = w.replace(append(w.buf, p...))
	if len(w.buf) <= w.hold {
		return n, nil
	}
	cut := len(w.buf) - w.hold
	if _, err := w.dst.Write(w.buf[:cut]); err != nil {
		return n, err
	}
	w.buf = append(w.buf[:0], w.buf[cut:]...)
	return n, nil
}

func (w *pathMapWriter) Flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	_, err := w.dst.Write(w.replace(w.buf))
	w.buf = w.buf[:0]
	return err
}

func (w *pathMapWriter) replace(b []byte) []byte {
	for _, rep := range w.reps {
		b = bytes.ReplaceAll(b, rep.from, rep.to)
	}
	return b
}
