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

// The root module has no namespace: its built-ins are available unqualified.
func init() {
	module.Register(module.BuiltinModule{Builtins: []*eval.Builtin{
		builtin("size", valueParam, size),
		builtin("empty", valueParam, empty),
		builtin("nempty", valueParam, nempty),
		builtin("prune", valueParam, prune),
		builtin("render", append(params("value"), optional("format", &ast.Null{})), renderValue),
		builtin("fail", params("message"), fail),
	}})
}

// fail stops the render with message, reported at the message itself.
func fail(c call) (eval.Value, error) {
	msg, err := c.str(0)
	if err != nil {
		return nil, err
	}
	return nil, eval.Errorf(c.pos(0), "%s", msg)
}

// size counts the items of a sequence, the entries of a mapping, or the
// characters of a string. Hidden entries count: size describes the value,
// not what the renderer will write out.
func size(c call) (eval.Value, error) {
	v, err := c.value(0)
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
		return nil, c.errorf(0, "needs a sequence, mapping or string, found %s", v.TypeName())
	}
}

func empty(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	return eval.Bool(isEmpty(v)), nil
}

func nempty(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	if isEmpty(v) {
		return eval.Null{}, nil
	}
	return v, nil
}

func prune(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	return pruned(v)
}

// pruned answers v without the empty items and entries at any depth. A
// collection is pruned before it is judged, so one that held only empty
// values is left out as well.
func pruned(v eval.Value) (eval.Value, error) {
	switch t := v.(type) {
	case *eval.Array:
		var items []*eval.Elem
		for _, item := range t.Items() {
			v, err := item.Value.Value()
			if err != nil {
				return nil, err
			}
			if v, err = pruned(v); err != nil {
				return nil, err
			}
			if !isEmpty(v) {
				items = append(items, &eval.Elem{Comments: item.Comments, Value: eval.Done(v)})
			}
		}
		return eval.NewArray(items), nil
	case *eval.Object:
		out := eval.NewObject()
		for _, f := range t.Fields() {
			v, err := f.Value.Value()
			if err != nil {
				return nil, err
			}
			if v, err = pruned(v); err != nil {
				return nil, err
			}
			if !isEmpty(v) {
				kept := *f
				kept.Value = eval.Done(v)
				out.Add(&kept)
			}
		}
		return out, nil
	default:
		return v, nil
	}
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
