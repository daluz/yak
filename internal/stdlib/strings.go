package stdlib

import (
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The strings module holds the operations on strings. Lengths are in
// characters rather than bytes, as they are for size.
func init() {
	module.Register(module.BuiltinModule{Path: "std.strings", Builtins: []*eval.Builtin{
		{Name: "upper", Params: valueParam, Fn: mapString("upper", strings.ToUpper)},
		{Name: "lower", Params: valueParam, Fn: mapString("lower", strings.ToLower)},
		{Name: "trim", Params: append(params("value"), optional("characters", &ast.Null{})), Fn: trim},
		{Name: "trimPrefix", Params: params("value", "prefix"), Fn: mapPair("trimPrefix", strings.TrimPrefix)},
		{Name: "trimSuffix", Params: params("value", "suffix"), Fn: mapPair("trimSuffix", strings.TrimSuffix)},
		{Name: "startsWith", Params: params("value", "prefix"), Fn: testPair("startsWith", strings.HasPrefix)},
		{Name: "endsWith", Params: params("value", "suffix"), Fn: testPair("endsWith", strings.HasSuffix)},
		{Name: "replace", Params: params("value", "old", "new"), Fn: replace},
		{Name: "repeat", Params: params("value", "count"), Fn: repeat},
		{Name: "truncate", Params: params("value", "length"), Fn: truncate},
		{Name: "split", Params: append(params("value"), optional("separator", &ast.Null{})), Fn: split},
		{Name: "join", Params: params("values", "separator"), Fn: join},
	}})
}

// mapString makes a built-in that turns one string into another.
func mapString(name string, fn func(string) string) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		s, err := stringArg(name, args[0])
		if err != nil {
			return nil, err
		}
		return eval.String(fn(s)), nil
	}
}

// mapPair makes a built-in that turns a string into another using a second
// string.
func mapPair(name string, fn func(string, string) string) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		s, t, err := stringPair(name, args)
		if err != nil {
			return nil, err
		}
		return eval.String(fn(s, t)), nil
	}
}

// testPair makes a built-in that answers a question about a string and a
// second string.
func testPair(name string, fn func(string, string) bool) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		s, t, err := stringPair(name, args)
		if err != nil {
			return nil, err
		}
		return eval.Bool(fn(s, t)), nil
	}
}

func replace(args []*eval.Thunk) (eval.Value, error) {
	s, old, err := stringPair("replace", args)
	if err != nil {
		return nil, err
	}
	repl, err := stringArg("replace", args[2])
	if err != nil {
		return nil, err
	}
	return eval.String(strings.ReplaceAll(s, old, repl)), nil
}

func repeat(args []*eval.Thunk) (eval.Value, error) {
	s, err := stringArg("repeat", args[0])
	if err != nil {
		return nil, err
	}
	n, err := countArg("repeat", args[1])
	if err != nil {
		return nil, err
	}
	return eval.String(strings.Repeat(s, n)), nil
}

// truncate keeps at most length characters of a string.
func truncate(args []*eval.Thunk) (eval.Value, error) {
	s, err := stringArg("truncate", args[0])
	if err != nil {
		return nil, err
	}
	n, err := countArg("truncate", args[1])
	if err != nil {
		return nil, err
	}
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}
	return eval.String(s), nil
}

// trim removes whitespace from either end of a string, or any of the given
// characters when there are some.
func trim(args []*eval.Thunk) (eval.Value, error) {
	s, err := stringArg("trim", args[0])
	if err != nil {
		return nil, err
	}
	chars, given, err := optionalString("trim", args[1])
	if err != nil {
		return nil, err
	}
	if !given {
		return eval.String(strings.TrimSpace(s)), nil
	}
	return eval.String(strings.Trim(s, chars)), nil
}

// split cuts a string around every separator. An empty separator splits it
// into its characters, and an empty string splits into nothing. Without a
// separator it cuts around every run of whitespace, leaving out whitespace
// at either end.
func split(args []*eval.Thunk) (eval.Value, error) {
	s, err := stringArg("split", args[0])
	if err != nil {
		return nil, err
	}
	sep, given, err := optionalString("split", args[1])
	if err != nil {
		return nil, err
	}
	var parts []string
	switch {
	case !given:
		parts = strings.Fields(s)
	case s != "":
		parts = strings.Split(s, sep)
	}
	items := make([]*eval.Elem, len(parts))
	for i, part := range parts {
		items[i] = eval.Item(eval.String(part))
	}
	return eval.NewArray(items), nil
}

// optionalString reads an argument that is a string, or null when it was
// left out.
func optionalString(name string, arg *eval.Thunk) (string, bool, error) {
	v, err := arg.Value()
	if err != nil {
		return "", false, err
	}
	if _, none := v.(eval.Null); none {
		return "", false, nil
	}
	s, err := stringArg(name, arg)
	return s, err == nil, err
}

// join is the reverse of split. Its items have to be strings already; one
// that is not is reported rather than converted, so that joining never hides
// a value the template did not mean to write.
func join(args []*eval.Thunk) (eval.Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	a, ok := v.(*eval.Array)
	if !ok {
		return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence of strings, found %s", "join", v.TypeName())
	}
	sep, err := stringArg("join", args[1])
	if err != nil {
		return nil, err
	}
	parts := make([]string, a.Len())
	for i, item := range a.Items() {
		v, err := item.Value.Value()
		if err != nil {
			return nil, err
		}
		s, ok := v.(eval.String)
		if !ok {
			return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence of strings, found %s at index %d",
				"join", v.TypeName(), i)
		}
		parts[i] = string(s)
	}
	return eval.String(strings.Join(parts, sep)), nil
}

func stringArg(name string, arg *eval.Thunk) (string, error) {
	v, err := arg.Value()
	if err != nil {
		return "", err
	}
	s, ok := v.(eval.String)
	if !ok {
		return "", eval.Errorf(arg.Pos(), "%q needs a string, found %s", name, v.TypeName())
	}
	return string(s), nil
}

func stringPair(name string, args []*eval.Thunk) (string, string, error) {
	s, err := stringArg(name, args[0])
	if err != nil {
		return "", "", err
	}
	t, err := stringArg(name, args[1])
	if err != nil {
		return "", "", err
	}
	return s, t, nil
}

// countArg reads an argument that says how many of something there are.
func countArg(name string, arg *eval.Thunk) (int, error) {
	v, err := arg.Value()
	if err != nil {
		return 0, err
	}
	n, ok := v.(eval.Int)
	if !ok {
		return 0, eval.Errorf(arg.Pos(), "%q needs an integer, found %s", name, v.TypeName())
	}
	if n < 0 {
		return 0, eval.Errorf(arg.Pos(), "%q needs a count of zero or more, found %d", name, int64(n))
	}
	return int(n), nil
}
