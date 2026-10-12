package stdlib

import (
	"fmt"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/token"
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

// valuesParam is the single parameter of the built-ins that take a sequence.
var valuesParam = params("values")

// builtin declares the built-in name, whose fn reads its arguments through
// the call it is handed.
func builtin(name string, ps []*ast.Param, fn func(call) (eval.Value, error)) *eval.Builtin {
	return &eval.Builtin{Name: name, Params: ps, Fn: func(args []*eval.Thunk) (eval.Value, error) {
		return fn(call{name: name, args: args})
	}}
}

// call is a call to the built-in name, holding one argument per parameter in
// the order they were declared. A complaint about an argument names the
// built-in and points at the argument.
type call struct {
	name string
	args []*eval.Thunk
}

func (c call) pos(i int) token.Pos { return c.args[i].Pos() }

func (c call) value(i int) (eval.Value, error) { return c.args[i].Value() }

// errorf reports a problem with argument i, after the name of the built-in.
func (c call) errorf(i int, format string, args ...any) error {
	return eval.Errorf(c.pos(i), "%q %s", c.name, fmt.Sprintf(format, args...))
}

func (c call) str(i int) (string, error) {
	v, err := c.value(i)
	if err != nil {
		return "", err
	}
	s, ok := v.(eval.String)
	if !ok {
		return "", c.errorf(i, "needs a string, found %s", v.TypeName())
	}
	return string(s), nil
}

// optionalStr reads an argument that is a string, or null when it was left
// out, reporting whether it was given.
func (c call) optionalStr(i int) (string, bool, error) {
	v, err := c.value(i)
	if err != nil {
		return "", false, err
	}
	if _, none := v.(eval.Null); none {
		return "", false, nil
	}
	s, err := c.str(i)
	return s, err == nil, err
}

func (c call) integer(i int) (eval.Int, error) {
	v, err := c.value(i)
	if err != nil {
		return 0, err
	}
	n, ok := v.(eval.Int)
	if !ok {
		return 0, c.errorf(i, "needs an integer, found %s", v.TypeName())
	}
	return n, nil
}

// number reads an argument that has to be an integer or a float.
func (c call) number(i int) (eval.Value, error) {
	v, err := c.value(i)
	if err != nil {
		return nil, err
	}
	switch v.(type) {
	case eval.Int, eval.Float:
		return v, nil
	}
	return nil, c.errorf(i, "needs a number, found %s", v.TypeName())
}

// count reads an argument that says how many of something there are.
func (c call) count(i int) (int, error) {
	n, err := c.integer(i)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, c.errorf(i, "needs a count of zero or more, found %d", int64(n))
	}
	return int(n), nil
}

// seq reads an argument that has to be a sequence, along with the values of
// its items.
func (c call) seq(i int) (*eval.Array, []eval.Value, error) {
	v, err := c.value(i)
	if err != nil {
		return nil, nil, err
	}
	a, ok := v.(*eval.Array)
	if !ok {
		return nil, nil, c.errorf(i, "needs a sequence, found %s", v.TypeName())
	}
	values := make([]eval.Value, a.Len())
	for j, item := range a.Items() {
		if values[j], err = item.Value.Value(); err != nil {
			return nil, nil, err
		}
	}
	return a, values, nil
}

// strs reads an argument that has to be a sequence of strings. An item that
// is not a string is reported rather than converted.
func (c call) strs(i int) ([]string, error) {
	v, err := c.value(i)
	if err != nil {
		return nil, err
	}
	a, ok := v.(*eval.Array)
	if !ok {
		return nil, c.errorf(i, "needs a sequence of strings, found %s", v.TypeName())
	}
	out := make([]string, a.Len())
	for j, item := range a.Items() {
		v, err := item.Value.Value()
		if err != nil {
			return nil, err
		}
		s, ok := v.(eval.String)
		if !ok {
			return nil, c.errorf(i, "needs a sequence of strings, found %s at index %d", v.TypeName(), j)
		}
		out[j] = string(s)
	}
	return out, nil
}

func (c call) mapping(i int) (*eval.Object, error) {
	v, err := c.value(i)
	if err != nil {
		return nil, err
	}
	o, ok := v.(*eval.Object)
	if !ok {
		return nil, c.errorf(i, "needs a mapping, found %s", v.TypeName())
	}
	return o, nil
}
