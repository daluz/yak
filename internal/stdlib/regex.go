package stdlib

import (
	"regexp"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The regex module matches strings against regular expressions in RE2
// syntax, which runs in time linear in its input whatever the pattern. A
// pattern matches anywhere in a string unless it anchors itself with ^ or $.
func init() {
	module.Register(module.BuiltinModule{Path: "std.regex", Builtins: []*eval.Builtin{
		{Name: "match", Params: params("value", "pattern"), Fn: match},
		{Name: "find", Params: params("value", "pattern"), Fn: find},
		{Name: "findAll", Params: params("value", "pattern"), Fn: findAll},
		{Name: "replace", Params: params("value", "pattern", "replacement"), Fn: replaceRegex},
	}})
}

// regexArgs reads a string and the pattern to match it against.
func regexArgs(name string, args []*eval.Thunk) (string, *regexp.Regexp, error) {
	s, pattern, err := stringPair(name, args)
	if err != nil {
		return "", nil, err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", nil, eval.Errorf(args[1].Pos(), "%q cannot compile its pattern: %s", name, err.Error())
	}
	return s, re, nil
}

func match(args []*eval.Thunk) (eval.Value, error) {
	s, re, err := regexArgs("match", args)
	if err != nil {
		return nil, err
	}
	return eval.Bool(re.MatchString(s)), nil
}

// find answers the first text that matches, or null when none does.
func find(args []*eval.Thunk) (eval.Value, error) {
	s, re, err := regexArgs("find", args)
	if err != nil {
		return nil, err
	}
	at := re.FindStringIndex(s)
	if at == nil {
		return eval.Null{}, nil
	}
	return eval.String(s[at[0]:at[1]]), nil
}

func findAll(args []*eval.Thunk) (eval.Value, error) {
	s, re, err := regexArgs("findAll", args)
	if err != nil {
		return nil, err
	}
	var items []*eval.Elem
	for _, m := range re.FindAllString(s, -1) {
		items = append(items, eval.Item(eval.String(m)))
	}
	return eval.NewArray(items), nil
}

// replaceRegex replaces every match. The replacement may name the groups of
// the pattern, as $1 or ${name}, and $$ writes a dollar sign.
func replaceRegex(args []*eval.Thunk) (eval.Value, error) {
	s, re, err := regexArgs("replace", args)
	if err != nil {
		return nil, err
	}
	repl, err := stringArg("replace", args[2])
	if err != nil {
		return nil, err
	}
	return eval.String(re.ReplaceAllString(s, repl)), nil
}
