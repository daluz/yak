// Package stdlib holds the built-in modules of the standard library, which
// are written in Go. Each one registers itself with the module package when
// this package is imported.
package stdlib

import (
	"unicode/utf8"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// params declares parameters that every call has to supply.
func params(names ...string) []*ast.Param {
	ps := make([]*ast.Param, len(names))
	for i, name := range names {
		ps[i] = &ast.Param{Name: name}
	}
	return ps
}

// optional declares a parameter that a call may leave out, and the literal
// it takes then.
func optional(name string, def ast.Node) *ast.Param {
	return &ast.Param{Name: name, Default: def}
}

// valueParam is the single parameter shared by the built-ins that take one
// thing and answer something about it.
var valueParam = params("value")

// The root module has no namespace: its built-ins are available unqualified.
func init() {
	module.Register(module.BuiltinModule{Builtins: []*eval.Builtin{
		{Name: "size", Params: valueParam, Fn: size},
		{Name: "empty", Params: valueParam, Fn: empty},
		{Name: "nempty", Params: valueParam, Fn: nempty},
	}})
}

// size counts the items of a sequence, the entries of a mapping, or the
// characters of a string. Hidden entries count: size describes the value,
// not what the renderer will write out.
func size(args []*eval.Thunk) (eval.Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.String:
		return eval.Int(utf8.RuneCountInString(string(t))), nil
	case *eval.Array:
		return eval.Int(t.Len()), nil
	case *eval.Object:
		return eval.Int(t.Len()), nil
	default:
		return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence, mapping or string, found %s",
			"size", v.TypeName())
	}
}

func empty(args []*eval.Thunk) (eval.Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	return eval.Bool(isEmpty(v)), nil
}

func nempty(args []*eval.Thunk) (eval.Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	if isEmpty(v) {
		return eval.Null{}, nil
	}
	return v, nil
}

// isEmpty reports whether a value holds nothing. Only null and the three
// empty collections do; a number or a boolean is a value like any other, as
// it is for "??" and ":?".
func isEmpty(v eval.Value) bool {
	switch t := v.(type) {
	case eval.Null:
		return true
	case eval.String:
		return t == ""
	case *eval.Array:
		return t.Len() == 0
	case *eval.Object:
		return t.Len() == 0
	default:
		return false
	}
}
