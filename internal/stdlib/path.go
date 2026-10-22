package stdlib

import (
	"path"
	"strings"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The path module takes apart and builds slash-separated paths, which is how
// std.files reads them on every system.
func init() {
	module.Register(module.BuiltinModule{Path: "std.path", Builtins: []*eval.Builtin{
		builtin("dir", valueParam, mapString(path.Dir)),
		builtin("base", valueParam, mapString(path.Base)),
		builtin("ext", valueParam, mapString(path.Ext)),
		builtin("stem", valueParam, mapString(stem)),
		builtin("clean", valueParam, mapString(path.Clean)),
		builtin("isAbs", valueParam, isAbs),
		builtin("join", valuesParam, joinPath),
	}})
}

// stem is the last element of a path without its extension.
func stem(p string) string {
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}

func isAbs(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	return eval.Bool(path.IsAbs(s)), nil
}

// joinPath joins a sequence of path elements with slashes and cleans the
// result, leaving out the empty ones.
func joinPath(c call) (eval.Value, error) {
	parts, err := c.strs(0)
	if err != nil {
		return nil, err
	}
	return eval.String(path.Join(parts...)), nil
}
