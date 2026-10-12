package stdlib

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The files module reads text files. A path is relative to the directory of
// the file that calls read, and in strict mode it cannot leave that
// directory.
func init() {
	module.Register(module.BuiltinModule{
		Path:           "std.files",
		Builtins:       []*eval.Builtin{builtin("read", params("path"), readFile(false))},
		StrictBuiltins: []*eval.Builtin{builtin("read", params("path"), readFile(true))},
	})
}

func readFile(strict bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		p, err := c.str(0)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(c.pos(0).File)
		var b []byte
		if strict {
			b, err = readUnder(c, dir, p)
		} else {
			full := filepath.FromSlash(p)
			if !filepath.IsAbs(full) {
				full = filepath.Join(dir, full)
			}
			b, err = os.ReadFile(full)
		}
		if err != nil {
			var e *eval.Error
			if errors.As(err, &e) {
				return nil, err
			}
			var pe *fs.PathError
			if errors.As(err, &pe) {
				err = pe.Err
			}
			return nil, c.errorf(0, "cannot read %q: %s", p, err.Error())
		}
		if !utf8.Valid(b) {
			return nil, c.errorf(0, "read %q, which is not UTF-8 text", p)
		}
		return eval.String(b), nil
	}
}

// readUnder reads the file at p beneath dir, refusing a path that leaves
// dir, whether by "..", by being absolute, or through a symbolic link.
func readUnder(c call, dir, p string) ([]byte, error) {
	local := filepath.FromSlash(p)
	if !filepath.IsLocal(local) {
		return nil, c.errorf(0,
			"cannot read %q in strict mode, which reads only beneath the directory of the file calling it; render with --strict=false to read anywhere",
			p)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(local)
}
