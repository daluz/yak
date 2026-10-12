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
		builtin("match", params("value", "pattern"), match),
		builtin("find", params("value", "pattern"), find),
		builtin("findAll", params("value", "pattern"), findAll),
		builtin("replace", params("value", "pattern", "replacement"), replaceRegex),
	}})
}

// regexArgs reads a string and the pattern to match it against.
func regexArgs(c call) (string, *regexp.Regexp, error) {
	s, pattern, err := stringPair(c)
	if err != nil {
		return "", nil, err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", nil, c.errorf(1, "cannot compile its pattern: %s", err.Error())
	}
	return s, re, nil
}

func match(c call) (eval.Value, error) {
	s, re, err := regexArgs(c)
	if err != nil {
		return nil, err
	}
	return eval.Bool(re.MatchString(s)), nil
}

// find answers the first text that matches, or null when none does.
func find(c call) (eval.Value, error) {
	s, re, err := regexArgs(c)
	if err != nil {
		return nil, err
	}
	at := re.FindStringIndex(s)
	if at == nil {
		return eval.Null{}, nil
	}
	return eval.String(s[at[0]:at[1]]), nil
}

func findAll(c call) (eval.Value, error) {
	s, re, err := regexArgs(c)
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
func replaceRegex(c call) (eval.Value, error) {
	s, re, err := regexArgs(c)
	if err != nil {
		return nil, err
	}
	repl, err := c.str(2)
	if err != nil {
		return nil, err
	}
	return eval.String(re.ReplaceAllString(s, repl)), nil
}
