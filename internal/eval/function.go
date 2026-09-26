package eval

import (
	"strconv"

	"github.com/daluz/yak/internal/ast"
)

// maxCallDepth bounds how deeply calls may nest, so that a function that ends
// up calling itself without end is reported rather than left to exhaust the
// stack. A cycle in a binding is caught by its thunk, but every call builds
// thunks of its own and so looks like fresh work each time.
const maxCallDepth = 1000

// Function is a binding that was declared with a parameter list, closed over
// the environment it was written in. It is a value like any other, so it may
// be passed to another function or held in a hidden field, but no output
// format can hold one.
type Function struct {
	decl *ast.Function
	env  *Env
}

// TypeName implements Value.
func (*Function) TypeName() string { return "function" }

func evalCall(node *ast.Call, env *Env) (Value, error) {
	target, err := evalNode(node.Fn, env)
	if err != nil {
		return nil, err
	}
	fn, ok := target.(*Function)
	if !ok {
		return nil, errorf(node.Fn.Pos(), "cannot call a value of type %s", target.TypeName())
	}
	return fn.call(node, env)
}

// call binds the arguments to the parameters and evaluates the body in the
// environment the function was declared in, so that a function sees what was
// in scope where it was written rather than where it is called.
//
// The arguments stay behind thunks of the calling environment, and the
// parameters share one frame, so a default may name another parameter.
func (f *Function) call(node *ast.Call, env *Env) (Value, error) {
	doc := f.env.doc
	if doc.depth >= maxCallDepth {
		return nil, errorf(node.Pos(), "function %q is nested more than %d calls deep and may not terminate",
			f.decl.Name, maxCallDepth)
	}
	args, err := f.arguments(node, env)
	if err != nil {
		return nil, err
	}
	s := &scope{outer: f.env.scope, binds: make(map[string]*Thunk, len(f.decl.Params))}
	inner := &Env{doc: f.env.doc, self: f.env.self, scope: s}
	for _, p := range f.decl.Params {
		switch t, ok := args[p.Name]; {
		case ok:
			s.binds[p.Name] = t
		case p.Default != nil:
			s.binds[p.Name] = &Thunk{node: p.Default, env: inner, pos: p.Default.Pos()}
		default:
			return nil, errorf(node.Pos(), "function %q needs an argument for parameter %q",
				f.decl.Name, p.Name)
		}
	}
	doc.depth++
	defer func() { doc.depth-- }()
	return evalNode(f.decl.Body, inner)
}

// arguments matches the arguments of a call to the names of the parameters
// they supply.
func (f *Function) arguments(node *ast.Call, env *Env) (map[string]*Thunk, error) {
	params := f.decl.Params
	args := make(map[string]*Thunk, len(node.Args))
	for i, arg := range node.Args {
		name := arg.Name
		switch {
		case name == "" && i >= len(params):
			return nil, errorf(arg.Value.Pos(), "function %q takes at most %s, found %d",
				f.decl.Name, argumentCount(len(params)), len(node.Args))
		case name == "":
			name = params[i].Name
		case f.param(name) == nil:
			return nil, errorf(arg.NamePos, "function %q has no parameter named %q%s",
				f.decl.Name, name, f.parameterNames())
		}
		if _, dup := args[name]; dup {
			return nil, errorf(arg.Value.Pos(), "parameter %q of function %q is given twice",
				name, f.decl.Name)
		}
		args[name] = &Thunk{node: arg.Value, env: env, pos: arg.Value.Pos()}
	}
	return args, nil
}

// argumentCount counts arguments for a diagnostic.
func argumentCount(n int) string {
	if n == 1 {
		return "1 argument"
	}
	return strconv.Itoa(n) + " arguments"
}

func (f *Function) param(name string) *ast.Param {
	for _, p := range f.decl.Params {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// parameterNames lists the parameters so that a misspelled argument can show
// what was on offer.
func (f *Function) parameterNames() string {
	if len(f.decl.Params) == 0 {
		return " (it takes none)"
	}
	names := make([]string, len(f.decl.Params))
	for i, p := range f.decl.Params {
		names[i] = p.Name
	}
	return "; parameters: " + quoteNames(names)
}
