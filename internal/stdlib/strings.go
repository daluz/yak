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
		builtin("upper", valueParam, mapString(strings.ToUpper)),
		builtin("lower", valueParam, mapString(strings.ToLower)),
		builtin("trim", append(params("value"), optional("characters", &ast.Null{})), trim),
		builtin("trimPrefix", params("value", "prefix"), mapPair(strings.TrimPrefix)),
		builtin("trimSuffix", params("value", "suffix"), mapPair(strings.TrimSuffix)),
		builtin("startsWith", params("value", "prefix"), testPair(strings.HasPrefix)),
		builtin("endsWith", params("value", "suffix"), testPair(strings.HasSuffix)),
		builtin("replace", params("value", "old", "new"), replace),
		builtin("repeat", params("value", "count"), repeat),
		builtin("truncate", params("value", "length"), truncate),
		builtin("split", append(params("value"), optional("separator", &ast.Null{})), split),
		builtin("join", params("values", "separator"), join),
	}})
}

// mapString makes a built-in that turns one string into another.
func mapString(fn func(string) string) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		s, err := c.str(0)
		if err != nil {
			return nil, err
		}
		return eval.String(fn(s)), nil
	}
}

// mapPair makes a built-in that turns a string into another using a second
// string.
func mapPair(fn func(string, string) string) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		s, t, err := stringPair(c)
		if err != nil {
			return nil, err
		}
		return eval.String(fn(s, t)), nil
	}
}

// testPair makes a built-in that answers a question about a string and a
// second string.
func testPair(fn func(string, string) bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		s, t, err := stringPair(c)
		if err != nil {
			return nil, err
		}
		return eval.Bool(fn(s, t)), nil
	}
}

// stringPair reads the first two arguments, which are both strings.
func stringPair(c call) (string, string, error) {
	s, err := c.str(0)
	if err != nil {
		return "", "", err
	}
	t, err := c.str(1)
	if err != nil {
		return "", "", err
	}
	return s, t, nil
}

func replace(c call) (eval.Value, error) {
	s, old, err := stringPair(c)
	if err != nil {
		return nil, err
	}
	repl, err := c.str(2)
	if err != nil {
		return nil, err
	}
	return eval.String(strings.ReplaceAll(s, old, repl)), nil
}

func repeat(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	n, err := c.count(1)
	if err != nil {
		return nil, err
	}
	return eval.String(strings.Repeat(s, n)), nil
}

// truncate keeps at most length characters of a string.
func truncate(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	n, err := c.count(1)
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
func trim(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	chars, given, err := c.optionalStr(1)
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
func split(c call) (eval.Value, error) {
	s, err := c.str(0)
	if err != nil {
		return nil, err
	}
	sep, given, err := c.optionalStr(1)
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

// join is the reverse of split. Its items have to be strings already; one
// that is not is reported rather than converted, so that joining never hides
// a value the template did not mean to write.
func join(c call) (eval.Value, error) {
	parts, err := c.strs(0)
	if err != nil {
		return nil, err
	}
	sep, err := c.str(1)
	if err != nil {
		return nil, err
	}
	return eval.String(strings.Join(parts, sep)), nil
}
