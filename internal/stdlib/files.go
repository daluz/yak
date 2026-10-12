package stdlib

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/token"
)

// The files module reads text files. A path is relative to the directory of
// the file that calls read, and in strict mode it cannot leave that
// directory.
func init() {
	module.Register(module.BuiltinModule{
		Path:           "std.files",
		Builtins:       []*eval.Builtin{{Name: "read", Params: params("path"), Fn: readFile(false)}},
		StrictBuiltins: []*eval.Builtin{{Name: "read", Params: params("path"), Fn: readFile(true)}},
	})
}

func readFile(strict bool) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		p, err := stringArg("read", args[0])
		if err != nil {
			return nil, err
		}
		pos := args[0].Pos()
		dir := filepath.Dir(pos.File)
		var b []byte
		if strict {
			b, err = readUnder(dir, p, pos)
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
			return nil, eval.Errorf(pos, "%q cannot read %q: %s", "read", p, err.Error())
		}
		if !utf8.Valid(b) {
			return nil, eval.Errorf(pos, "%q read %q, which is not UTF-8 text", "read", p)
		}
		return eval.String(b), nil
	}
}

// readUnder reads the file at p beneath dir, refusing a path that leaves
// dir, whether by "..", by being absolute, or through a symbolic link.
func readUnder(dir, p string, pos token.Pos) ([]byte, error) {
	local := filepath.FromSlash(p)
	if !filepath.IsLocal(local) {
		return nil, eval.Errorf(pos,
			"%q cannot read %q in strict mode, which reads only beneath the directory of the file calling it; render with --strict=false to read anywhere",
			"read", p)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(local)
}
