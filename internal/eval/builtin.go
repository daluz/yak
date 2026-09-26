package eval

import (
	"unicode/utf8"

	"github.com/daluz/yak/internal/ast"
)

// Builtin is a function of the standard library, written in Go rather than in
// a template. It is a value like a user function and is called the same way,
// so it can be passed around as one, but every parameter it declares is
// required: a built-in has no defaults.
type Builtin struct {
	name   string
	params []*ast.Param
	// fn receives one thunk per parameter, in the order they were declared,
	// so that it decides what to resolve and can point a complaint at the
	// argument that caused it.
	fn func(args []*Thunk) (Value, error)
}

// TypeName implements Value. A built-in is a function as far as the rest of
// the language is concerned, output formats included.
func (*Builtin) TypeName() string { return "function" }

func (b *Builtin) call(node *ast.Call, env *Env) (Value, error) {
	matched, err := matchArguments(b.name, b.params, node, env)
	if err != nil {
		return nil, err
	}
	args := make([]*Thunk, len(b.params))
	for i, p := range b.params {
		t, ok := matched[p.Name]
		if !ok {
			return nil, missingArgument(b.name, p.Name, node)
		}
		args[i] = t
	}
	return b.fn(args)
}

// valueParam is the single parameter shared by the built-ins that take one
// thing and answer something about it.
var valueParam = []*ast.Param{{Name: "value"}}

// builtins are the standard library functions that are available unqualified.
// They are found only after every binding in scope, so a local of the same
// name shadows one rather than colliding with it.
//
// The table is filled in at startup because a built-in evaluates its
// arguments, so it and the evaluator refer to each other.
var builtins = map[string]*Builtin{}

func init() {
	for _, b := range []*Builtin{
		{name: "size", params: valueParam, fn: builtinSize},
		{name: "empty", params: valueParam, fn: builtinEmpty},
		{name: "nullify", params: valueParam, fn: builtinNullify},
	} {
		builtins[b.name] = b
	}
}

// builtinSize counts the items of a sequence, the entries of a mapping, or
// the characters of a string. Hidden entries count: size describes the value,
// not what the renderer will write out.
func builtinSize(args []*Thunk) (Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case String:
		return Int(utf8.RuneCountInString(string(t))), nil
	case *Array:
		return Int(t.Len()), nil
	case *Object:
		return Int(t.Len()), nil
	default:
		return nil, errorf(args[0].Pos(), "%q needs a sequence, mapping or string, found %s",
			"size", v.TypeName())
	}
}

func builtinEmpty(args []*Thunk) (Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	return Bool(isEmpty(v)), nil
}

func builtinNullify(args []*Thunk) (Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	if isEmpty(v) {
		return Null{}, nil
	}
	return v, nil
}

// isEmpty reports whether a value holds nothing. Only null and the three
// empty collections do; a number or a boolean is a value like any other, as
// it is for "??" and ":?".
func isEmpty(v Value) bool {
	switch t := v.(type) {
	case Null:
		return true
	case String:
		return t == ""
	case *Array:
		return t.Len() == 0
	case *Object:
		return t.Len() == 0
	default:
		return false
	}
}
