package eval

import (
	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// Builtin is a function of the standard library, written in Go rather than in
// a template. It is a value like a user function and is called the same way,
// so it can be passed around as one.
type Builtin struct {
	Name string
	// Params are declared as a function's are. A built-in has no scope to
	// evaluate a default in, so a default is a literal with no
	// interpolation.
	Params []*ast.Param
	// Fn receives one thunk per parameter, in the order they were declared,
	// so that it decides what to resolve and can point a complaint at the
	// argument that caused it.
	Fn func(args []*Thunk) (Value, error)
}

// A built-in is a function as far as the rest of the language is concerned,
// output formats included.
func (*Builtin) TypeName() string { return "function" }

func (b *Builtin) call(node *ast.Call, env *Env) (Value, error) {
	matched, err := matchArguments(callee{name: b.Name}, b.Params, node, env)
	if err != nil {
		return nil, err
	}
	return b.run(node.Pos(), matched)
}

// run calls Fn with args in the order of the parameters they name, and the
// default of every parameter the call at pos left out.
func (b *Builtin) run(pos token.Pos, args map[string]*Thunk) (Value, error) {
	ordered := make([]*Thunk, len(b.Params))
	for i, p := range b.Params {
		switch t, ok := args[p.Name]; {
		case ok:
			ordered[i] = t
		case p.Default != nil:
			v, err := evalNode(p.Default, nil)
			if err != nil {
				return nil, err
			}
			ordered[i] = &Thunk{state: resolved, val: v, pos: pos}
		default:
			return nil, missingArgument(callee{name: b.Name}, p.Name, pos)
		}
	}
	return b.Fn(ordered)
}

// Errorf reports a failure at pos, which is how a built-in complains about
// one of its arguments.
func Errorf(pos token.Pos, format string, args ...any) error {
	return errorf(pos, format, args...)
}
