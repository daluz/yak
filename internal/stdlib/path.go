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
		{Name: "dir", Params: valueParam, Fn: mapString("dir", path.Dir)},
		{Name: "base", Params: valueParam, Fn: mapString("base", path.Base)},
		{Name: "ext", Params: valueParam, Fn: mapString("ext", path.Ext)},
		{Name: "stem", Params: valueParam, Fn: mapString("stem", stem)},
		{Name: "clean", Params: valueParam, Fn: mapString("clean", path.Clean)},
		{Name: "isAbs", Params: valueParam, Fn: isAbs},
		{Name: "join", Params: params("values"), Fn: joinPath},
	}})
}

// stem is the last element of a path without its extension.
func stem(p string) string {
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}

func isAbs(args []*eval.Thunk) (eval.Value, error) {
	s, err := stringArg("isAbs", args[0])
	if err != nil {
		return nil, err
	}
	return eval.Bool(path.IsAbs(s)), nil
}

// joinPath joins a sequence of path elements with slashes and cleans the
// result, leaving out the empty ones.
func joinPath(args []*eval.Thunk) (eval.Value, error) {
	_, values, err := seqArg("join", args[0])
	if err != nil {
		return nil, err
	}
	parts := make([]string, len(values))
	for i, v := range values {
		s, ok := v.(eval.String)
		if !ok {
			return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence of strings, found %s at index %d",
				"join", v.TypeName(), i)
		}
		parts[i] = string(s)
	}
	return eval.String(path.Join(parts...)), nil
}
