package eval

import (
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// docState holds the per-document state shared by every environment inside
// that document.
type docState struct {
	root    *Thunk
	context Value
	// stack records the positions currently being evaluated so that a cycle
	// can be reported as the chain of references that formed it.
	stack []token.Pos
}

// Env is the lexical environment of an expression.
type Env struct {
	doc *docState
	// self holds the enclosing mappings, innermost first. Sequences do not
	// add a level, so ".." always names the nearest enclosing mapping.
	self []*Object
}

func (e *Env) pushSelf(o *Object) *Env {
	self := make([]*Object, 0, len(e.self)+1)
	self = append(self, o)
	self = append(self, e.self...)
	return &Env{doc: e.doc, self: self}
}

// Document evaluates a single document against the given context value. The
// returned value is fully constructed but its leaves are still lazy; use
// Force to resolve them.
func Document(doc *ast.Document, context Value) (Value, error) {
	if context == nil {
		context = NewObject()
	}
	ds := &docState{context: context}
	env := &Env{doc: ds}
	root := &Thunk{node: doc.Body, env: env, pos: doc.Pos()}
	ds.root = root
	return root.Value()
}

func evalNode(n ast.Node, env *Env) (Value, error) {
	switch node := n.(type) {
	case *ast.Mapping:
		return evalMapping(node, env)
	case *ast.Sequence:
		return evalSequence(node, env)
	case *ast.String:
		return evalString(node, env)
	case *ast.Int:
		return Int(node.Value), nil
	case *ast.Float:
		return Float(node.Value), nil
	case *ast.Bool:
		return Bool(node.Value), nil
	case *ast.Null:
		return Null{}, nil
	case *ast.Self:
		return evalSelf(node, env)
	case *ast.Root:
		return env.doc.root.Value()
	case *ast.Context:
		return env.doc.context, nil
	case *ast.Field:
		return evalField(node, env)
	case *ast.Index:
		return evalIndex(node, env)
	case *ast.Ident:
		return nil, errorf(node.Pos(),
			"unknown identifier %q; strings must be quoted, and %q bindings are not implemented yet",
			node.Name, "local")
	default:
		return nil, errorf(n.Pos(), "internal error: cannot evaluate %T", n)
	}
}

// evalMapping builds the mapping eagerly so that references into it resolve,
// while leaving each value behind a thunk.
//
// Keys are resolved in two passes. Literal keys are registered first so that a
// computed key may refer to a sibling; computed keys are resolved afterwards.
// Slots are reserved up front, which keeps the output in source order no
// matter which pass names them.
func evalMapping(node *ast.Mapping, env *Env) (Value, error) {
	obj := NewObject()
	child := env.pushSelf(obj)

	slots := make([]*Field, len(node.Entries))
	for i, e := range node.Entries {
		slots[i] = obj.reserve(e.Hidden, &Thunk{node: e.Value, env: child, pos: e.Value.Pos()})
	}
	for i, e := range node.Entries {
		if name, ok := literalKey(e); ok {
			if err := obj.bind(slots[i], name); err != nil {
				return nil, errorf(e.KeyPos, "%s", err.Error())
			}
		}
	}
	for i, e := range node.Entries {
		if _, ok := literalKey(e); ok {
			continue
		}
		key, err := evalNode(e.Key, child)
		if err != nil {
			return nil, err
		}
		name, ok := key.(String)
		if !ok {
			return nil, errorf(e.KeyPos, "mapping keys must be strings, found %s", key.TypeName())
		}
		if err := obj.bind(slots[i], string(name)); err != nil {
			return nil, errorf(e.KeyPos, "%s", err.Error())
		}
	}
	return obj, nil
}

// literalKey returns the name of a key that is known without evaluation.
func literalKey(e *ast.Entry) (string, bool) {
	if e.Computed {
		return "", false
	}
	s, ok := e.Key.(*ast.String)
	if !ok || !s.IsLiteral() {
		return "", false
	}
	return s.Literal(), true
}

func evalSequence(node *ast.Sequence, env *Env) (Value, error) {
	items := make([]*Thunk, len(node.Items))
	for i, item := range node.Items {
		items[i] = &Thunk{node: item, env: env, pos: item.Pos()}
	}
	return NewArray(items), nil
}

func evalString(node *ast.String, env *Env) (Value, error) {
	if node.IsLiteral() {
		return String(node.Literal()), nil
	}
	var sb strings.Builder
	for _, part := range node.Parts {
		if part.Expr == nil {
			sb.WriteString(part.Text)
			continue
		}
		v, err := evalNode(part.Expr, env)
		if err != nil {
			return nil, err
		}
		s, err := stringify(v)
		if err != nil {
			return nil, errorf(part.Expr.Pos(), "%s", err.Error())
		}
		sb.WriteString(s)
	}
	return String(sb.String()), nil
}

func evalSelf(node *ast.Self, env *Env) (Value, error) {
	if node.Up >= len(env.self) {
		return nil, errorf(node.Pos(), "%q reaches past the outermost mapping of the document",
			strings.Repeat(".", node.Up+1))
	}
	return env.self[node.Up], nil
}

func evalField(node *ast.Field, env *Env) (Value, error) {
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	obj, ok := x.(*Object)
	if !ok {
		return nil, errorf(node.Pos(), "cannot read field %q from a %s", node.Name, x.TypeName())
	}
	f, ok := obj.Lookup(node.Name)
	if !ok {
		return nil, errorf(node.Pos(), "no field %q in mapping%s", node.Name, availableFields(obj))
	}
	return f.Value.Value()
}

func evalIndex(node *ast.Index, env *Env) (Value, error) {
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	idx, err := evalNode(node.Index, env)
	if err != nil {
		return nil, err
	}
	switch container := x.(type) {
	case *Array:
		n, ok := idx.(Int)
		if !ok {
			return nil, errorf(node.Index.Pos(), "sequence index must be an integer, found %s", idx.TypeName())
		}
		i := int(n)
		if i < 0 {
			i += container.Len()
		}
		if i < 0 || i >= container.Len() {
			return nil, errorf(node.Index.Pos(), "index %d is out of range for a sequence of length %d", int(n), container.Len())
		}
		return container.Items()[i].Value()
	case *Object:
		name, ok := idx.(String)
		if !ok {
			return nil, errorf(node.Index.Pos(), "mapping index must be a string, found %s", idx.TypeName())
		}
		f, ok := container.Lookup(string(name))
		if !ok {
			return nil, errorf(node.Index.Pos(), "no field %q in mapping%s", string(name), availableFields(container))
		}
		return f.Value.Value()
	default:
		return nil, errorf(node.Pos(), "cannot index a %s", x.TypeName())
	}
}

// availableFields lists the field names of a mapping so that a failed lookup
// can show what was there instead.
func availableFields(o *Object) string {
	names := o.Names()
	if len(names) == 0 {
		return " (the mapping is empty)"
	}
	if len(names) > 8 {
		names = names[:8]
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = `"` + n + `"`
	}
	return "; available fields: " + strings.Join(quoted, ", ")
}
