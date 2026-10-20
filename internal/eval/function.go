package eval

import (
	"fmt"
	"slices"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// maxCallDepth bounds how deeply calls may nest, so that a function that ends
// up calling itself without end is reported rather than left to exhaust the
// stack. A cycle in a binding is caught by its thunk, but every call builds
// thunks of its own and so looks like fresh work each time.
const maxCallDepth = 1000

// function is a binding that was declared with a parameter list, closed over
// the environment it was written in. It is a value like any other, so it may
// be passed to another function or held in a hidden field, but no output
// format can hold one.
type function struct {
	decl *ast.Function
	env  *Env
}

func (*function) TypeName() string { return "function" }

// callee names the function a diagnostic is about. An anonymous function has
// no name to give, so it is placed by where it was written instead. The text
// is built only when something has gone wrong.
type callee struct {
	name string
	pos  token.Pos
}

func (c callee) String() string {
	if c.name == "" {
		return fmt.Sprintf("the function at %d:%d", c.pos.Line, c.pos.Col)
	}
	return fmt.Sprintf("function %q", c.name)
}

func evalCall(node *ast.Call, env *Env) (Value, error) {
	target, err := evalNode(node.Fn, env)
	if err != nil {
		return nil, err
	}
	switch fn := target.(type) {
	case *function:
		return fn.call(node, env)
	case *Builtin:
		return fn.call(node, env)
	default:
		return nil, errorf(node.Fn.Pos(), "cannot call a value of type %s", target.TypeName())
	}
}

// call binds the arguments to the parameters and evaluates the body in the
// environment the function was declared in, so that a function sees what was
// in scope where it was written rather than where it is called.
//
// The arguments stay behind thunks of the calling environment, and the
// parameters share one frame, so a default may name another parameter.
func (f *function) call(node *ast.Call, env *Env) (Value, error) {
	if err := f.checkDepth(node.Pos()); err != nil {
		return nil, err
	}
	args, err := matchArguments(f.callee(), f.decl.Params, node, env)
	if err != nil {
		return nil, err
	}
	return f.run(node.Pos(), args)
}

func (f *function) callee() callee {
	return callee{name: f.decl.Name, pos: f.decl.Pos()}
}

func (f *function) checkDepth(pos token.Pos) error {
	if f.env.doc.session.depth >= maxCallDepth {
		return errorf(pos, "%s is nested more than %d calls deep and may not terminate",
			f.callee(), maxCallDepth)
	}
	return nil
}

// run evaluates the body with args bound to the parameters they name. pos is
// the call, which a missing argument is reported at.
func (f *function) run(pos token.Pos, args map[string]*Thunk) (Value, error) {
	session := f.env.doc.session
	who := f.callee()
	s := &scope{outer: f.env.scope, binds: make(map[string]*Thunk, len(f.decl.Params))}
	inner := &Env{doc: f.env.doc, self: f.env.self, scope: s}
	for _, p := range f.decl.Params {
		t, ok := args[p.Label()]
		if !ok {
			if p.Default == nil {
				return nil, missingArgument(who, p.Label(), pos)
			}
			t = &Thunk{node: p.Default, env: inner, pos: p.Default.Pos()}
		}
		if p.Pattern == nil {
			s.binds[p.Name] = t
		} else {
			destructure(p.Pattern, t, inner, Comments{}, s.binds)
		}
	}
	session.depth++
	defer func() { session.depth-- }()
	return evalNode(f.decl.Body, inner)
}

// Call calls fn with arguments that are already values, which is how a
// built-in calls a function it was handed. The arguments go by position, and
// pos is where the call is reported, both for a mistake in the call and as
// the position of every argument.
func Call(fn Value, pos token.Pos, args ...Value) (Value, error) {
	thunks := make([]*Thunk, len(args))
	for i, a := range args {
		thunks[i] = &Thunk{state: resolved, val: a, pos: pos}
	}
	switch f := fn.(type) {
	case *function:
		if err := f.checkDepth(pos); err != nil {
			return nil, err
		}
		matched, err := byPosition(f.callee(), f.decl.Params, pos, thunks)
		if err != nil {
			return nil, err
		}
		return f.run(pos, matched)
	case *Builtin:
		matched, err := byPosition(callee{name: f.Name}, f.Params, pos, thunks)
		if err != nil {
			return nil, err
		}
		return f.run(pos, matched)
	default:
		return nil, errorf(pos, "cannot call a value of type %s", fn.TypeName())
	}
}

// byPosition matches arguments to the parameters they stand in for by
// position, as matchArguments does for a call written without names.
func byPosition(fn callee, params []*ast.Param, pos token.Pos, args []*Thunk) (map[string]*Thunk, error) {
	if len(args) > len(params) {
		return nil, errorf(pos, "%s takes at most %s, found %d", fn, plural(len(params), "argument"), len(args))
	}
	matched := make(map[string]*Thunk, len(args))
	for i, t := range args {
		matched[params[i].Label()] = t
	}
	return matched, nil
}

// matchArguments matches the arguments of a call to the names of the
// parameters they supply, leaving each one behind a thunk of the calling
// environment. A user function and a built-in are called the same way, so
// both go through here.
func matchArguments(fn callee, params []*ast.Param, node *ast.Call, env *Env) (map[string]*Thunk, error) {
	args := make(map[string]*Thunk, len(node.Args))
	for i, arg := range node.Args {
		name := arg.Name
		switch {
		case name == "" && i >= len(params):
			return nil, errorf(arg.Value.Pos(), "%s takes at most %s, found %d",
				fn, plural(len(params), "argument"), len(node.Args))
		case name == "":
			name = params[i].Label()
		case !slices.ContainsFunc(params, func(p *ast.Param) bool { return p.Name == name }):
			return nil, errorf(arg.NamePos, "%s has no parameter named %q%s",
				fn, name, parameterNames(params))
		}
		if _, dup := args[name]; dup {
			return nil, errorf(arg.Value.Pos(), "parameter %q of %s is given twice",
				name, fn)
		}
		args[name] = &Thunk{node: arg.Value, env: env, pos: arg.Value.Pos()}
	}
	return args, nil
}

// missingArgument reports a parameter that the call left out and that has no
// default to fall back on.
func missingArgument(fn callee, param string, pos token.Pos) error {
	return errorf(pos, "%s needs an argument for parameter %q", fn, param)
}

// parameterNames lists the parameters so that a misspelled argument can show
// what was on offer.
func parameterNames(params []*ast.Param) string {
	if len(params) == 0 {
		return " (it takes none)"
	}
	names := make([]string, len(params))
	for i, p := range params {
		names[i] = p.Label()
	}
	return "; parameters: " + quoteNames(names)
}
