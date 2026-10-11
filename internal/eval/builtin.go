package eval

import (
	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// Builtin is a function of the standard library, written in Go rather than in
// a template. It is a value like a user function and is called the same way,
// so it can be passed around as one, but every parameter it declares is
// required: a built-in has no defaults.
type Builtin struct {
	Name   string
	Params []string
	// Fn receives one thunk per parameter, in the order they were declared,
	// so that it decides what to resolve and can point a complaint at the
	// argument that caused it.
	Fn func(args []*Thunk) (Value, error)
}

// TypeName implements Value. A built-in is a function as far as the rest of
// the language is concerned, output formats included.
func (*Builtin) TypeName() string { return "function" }

func (b *Builtin) call(node *ast.Call, env *Env) (Value, error) {
	who := callee{name: b.Name}
	params := make([]*ast.Param, len(b.Params))
	for i, name := range b.Params {
		params[i] = &ast.Param{Name: name}
	}
	matched, err := matchArguments(who, params, node, env)
	if err != nil {
		return nil, err
	}
	args := make([]*Thunk, len(params))
	for i, p := range params {
		t, ok := matched[p.Name]
		if !ok {
			return nil, missingArgument(who, p.Name, node)
		}
		args[i] = t
	}
	return b.Fn(args)
}

// Errorf reports a failure at pos, which is how a built-in complains about
// one of its arguments.
func Errorf(pos token.Pos, format string, args ...any) error {
	return errorf(pos, format, args...)
}
