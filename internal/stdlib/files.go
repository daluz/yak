package stdlib

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The files module reads text files and directories. A path is relative to
// the directory of the file that calls the built-in, and in strict mode it
// cannot leave that directory.
func init() {
	module.Register(module.BuiltinModule{
		Path:           "std.files",
		Builtins:       filesBuiltins(false),
		StrictBuiltins: filesBuiltins(true),
	})
}

func filesBuiltins(strict bool) []*eval.Builtin {
	onlyFiles := optional("onlyFiles", &ast.Bool{Value: false})
	return []*eval.Builtin{
		builtin("read", params("path"), readFile(strict)),
		builtin("list", append(params("path"), onlyFiles), listFiles(strict)),
		builtin("glob", append(params("pattern"), onlyFiles), globFiles(strict)),
		builtin("isDir", params("path"), statPath(strict, func(fi fs.FileInfo) bool { return fi.IsDir() })),
		builtin("exists", params("path"), statPath(strict, func(fs.FileInfo) bool { return true })),
	}
}

func readFile(strict bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		p, fsys, err := resolve(c, strict, "read")
		if err != nil {
			return nil, err
		}
		defer fsys.close()
		b, err := fsys.ReadFile(osName(p))
		if err != nil {
			return nil, fileError(c, "read", p, err)
		}
		if !utf8.Valid(b) {
			return nil, c.errorf(0, "read %q, which is not UTF-8 text", p)
		}
		return eval.String(b), nil
	}
}

// listFiles answers the sorted names of the entries in a directory, or only
// of its regular files, following symbolic links.
func listFiles(strict bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		p, fsys, err := resolve(c, strict, "list")
		if err != nil {
			return nil, err
		}
		defer fsys.close()
		onlyFiles, err := c.boolean(1)
		if err != nil {
			return nil, err
		}
		entries, err := readDir(fsys, p)
		if err != nil {
			return nil, fileError(c, "list", p, err)
		}
		var names []string
		for _, e := range entries {
			if !onlyFiles || isRegular(fsys, path.Join(p, e.Name())) {
				names = append(names, e.Name())
			}
		}
		slices.Sort(names)
		return strArray(names), nil
	}
}

// globFiles answers the sorted paths that match a pattern, written as the
// pattern writes them. A segment of "**" matches any number of directories,
// without following symbolic links to them.
func globFiles(strict bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		p, fsys, err := resolve(c, strict, "match")
		if err != nil {
			return nil, err
		}
		defer fsys.close()
		onlyFiles, err := c.boolean(1)
		if err != nil {
			return nil, err
		}
		segs := strings.Split(p, "/")
		for _, seg := range segs {
			if _, err := path.Match(seg, ""); err != nil {
				return nil, c.errorf(0, "has a malformed pattern %q", p)
			}
		}
		g := globber{fsys: fsys, onlyFiles: onlyFiles, seen: map[string]bool{}}
		if path.IsAbs(p) {
			g.walk("/", segs[1:])
		} else {
			g.walk("", segs)
		}
		slices.Sort(g.matches)
		return strArray(g.matches), nil
	}
}

type globber struct {
	fsys      fileSystem
	onlyFiles bool
	seen      map[string]bool
	matches   []string
}

// walk matches segs against what is beneath dir, a path written with
// slashes and relative to the calling file unless it is absolute.
func (g *globber) walk(dir string, segs []string) {
	if len(segs) == 0 {
		g.add(dir)
		return
	}
	seg, rest := segs[0], segs[1:]
	switch {
	case seg == "**":
		if len(rest) > 0 {
			g.walk(dir, rest)
		}
		entries, _ := readDir(g.fsys, dir)
		for _, e := range entries {
			child := path.Join(dir, e.Name())
			if len(rest) == 0 {
				g.add(child)
			}
			if e.IsDir() {
				g.walk(child, segs)
			}
		}
	case !strings.ContainsAny(seg, `*?[\`):
		g.walk(path.Join(dir, seg), rest)
	default:
		entries, _ := readDir(g.fsys, dir)
		for _, e := range entries {
			if ok, _ := path.Match(seg, e.Name()); ok {
				g.walk(path.Join(dir, e.Name()), rest)
			}
		}
	}
}

func (g *globber) add(p string) {
	if p == "" || g.seen[p] {
		return
	}
	if g.onlyFiles && !isRegular(g.fsys, p) {
		return
	}
	if _, err := g.fsys.Stat(osName(p)); err != nil {
		return
	}
	g.seen[p] = true
	g.matches = append(g.matches, p)
}

// statPath answers whether a path exists and test holds for it.
func statPath(strict bool, test func(fs.FileInfo) bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		p, fsys, err := resolve(c, strict, "check")
		if err != nil {
			return nil, err
		}
		defer fsys.close()
		fi, err := fsys.Stat(osName(p))
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return eval.Bool(false), nil
		}
		if err != nil {
			return nil, fileError(c, "check", p, err)
		}
		return eval.Bool(test(fi)), nil
	}
}

func readDir(fsys fileSystem, p string) ([]fs.DirEntry, error) {
	f, err := fsys.Open(osName(p))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

// isRegular reports whether p is a regular file once symbolic links are
// followed; a link that cannot be followed is not.
func isRegular(fsys fileSystem, p string) bool {
	fi, err := fsys.Stat(osName(p))
	return err == nil && fi.Mode().IsRegular()
}

func strArray(ss []string) *eval.Array {
	items := make([]*eval.Elem, len(ss))
	for i, s := range ss {
		items[i] = eval.Item(eval.String(s))
	}
	return eval.NewArray(items)
}

// osName turns a path written with slashes into one for the file system.
func osName(p string) string {
	if p == "" {
		return "."
	}
	return filepath.FromSlash(p)
}

// fileSystem is where a built-in finds the paths it is given: beneath the
// directory of the calling file in strict mode, anywhere otherwise.
type fileSystem interface {
	Open(name string) (*os.File, error)
	Stat(name string) (fs.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	close()
}

type rootFS struct{ *os.Root }

func (r rootFS) close() { r.Close() }

// osFS finds a relative name in dir.
type osFS struct{ dir string }

func (o osFS) path(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(o.dir, name)
}

func (o osFS) Open(name string) (*os.File, error)    { return os.Open(o.path(name)) }
func (o osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(o.path(name)) }
func (o osFS) ReadFile(name string) ([]byte, error)  { return os.ReadFile(o.path(name)) }
func (osFS) close()                                  {}

// resolve reads the path argument of c and opens the directory of the
// calling file to find it in. In strict mode it refuses a path that leaves
// that directory, whether by "..", by being absolute, or through a symbolic
// link.
func resolve(c call, strict bool, verb string) (string, fileSystem, error) {
	p, err := c.str(0)
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Dir(c.pos(0).File)
	if !strict {
		return p, osFS{dir}, nil
	}
	if !filepath.IsLocal(filepath.FromSlash(p)) {
		return "", nil, c.errorf(0,
			"cannot %s %q in strict mode, which reads only beneath the directory of the file calling it; render with --strict=false to read anywhere",
			verb, p)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", nil, fileError(c, verb, p, err)
	}
	return p, rootFS{root}, nil
}

func fileError(c call, verb, p string, err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		err = pe.Err
	}
	return c.errorf(0, "cannot %s %q: %s", verb, p, err.Error())
}
