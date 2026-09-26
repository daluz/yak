package eval

import (
	"sort"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
	"github.com/daluz/yak/internal/version"
)

// docState holds the per-document state shared by every environment inside
// that document.
type docState struct {
	root    *Thunk
	context Value
	// yak is the mapping "$yak" answers. It describes the run, so every
	// document of a stream sees the same thing.
	yak Value
	// stack records the positions currently being evaluated so that a cycle
	// can be reported as the chain of references that formed it.
	stack []token.Pos
	// depth counts the function calls being evaluated, which bounds a
	// recursion that never ends.
	depth int
}

// Env is the lexical environment of an expression.
type Env struct {
	doc *docState
	// self holds the enclosing mappings, innermost first. Sequences do not
	// add a level, so ".." always names the nearest enclosing mapping.
	self []*Object
	// scope is the innermost frame of local bindings, or nil at the top of
	// a document.
	scope *scope
}

// scope is one frame of local bindings. Frames link outwards, so an inner
// binding shadows an outer one of the same name.
type scope struct {
	outer *scope
	binds map[string]*Thunk
}

func (e *Env) pushSelf(o *Object) *Env {
	self := make([]*Object, 0, len(e.self)+1)
	self = append(self, o)
	self = append(self, e.self...)
	return &Env{doc: e.doc, self: self, scope: e.scope}
}

// pushBindings returns an environment that adds a frame holding binds. The
// bindings are evaluated in that same environment, so they may refer to each
// other; a binding that ends up needing itself is reported as a cycle.
func (e *Env) pushBindings(binds []*ast.Binding) (*Env, error) {
	if len(binds) == 0 {
		return e, nil
	}
	s := &scope{outer: e.scope, binds: make(map[string]*Thunk, len(binds))}
	inner := &Env{doc: e.doc, self: e.self, scope: s}
	for _, b := range binds {
		if _, ok := s.binds[b.Name]; ok {
			return nil, errorf(b.Pos(), "duplicate binding %q in the same scope", b.Name)
		}
		s.binds[b.Name] = &Thunk{node: b.Value, env: inner, pos: b.Value.Pos()}
	}
	return inner, nil
}

// bindOne returns an environment that adds a frame holding a single binding,
// which is how a comprehension names the item it is looking at.
func (e *Env) bindOne(name string, t *Thunk) *Env {
	s := &scope{outer: e.scope, binds: map[string]*Thunk{name: t}}
	return &Env{doc: e.doc, self: e.self, scope: s}
}

// lookup finds a binding, searching outwards from the innermost frame.
func (e *Env) lookup(name string) (*Thunk, bool) {
	for s := e.scope; s != nil; s = s.outer {
		if t, ok := s.binds[name]; ok {
			return t, true
		}
	}
	return nil, false
}

// bindingNames lists every binding visible here, innermost first and without
// the names an inner frame has shadowed.
func (e *Env) bindingNames() []string {
	var names []string
	seen := map[string]bool{}
	for s := e.scope; s != nil; s = s.outer {
		frame := make([]string, 0, len(s.binds))
		for name := range s.binds {
			if !seen[name] {
				seen[name] = true
				frame = append(frame, name)
			}
		}
		sort.Strings(frame)
		names = append(names, frame...)
	}
	return names
}

// Run describes the rendering itself, which the language reads as "$yak".
type Run struct {
	// FilePath is the template as the command line named it. Input read
	// from standard input is called "<stdin>", as it is in diagnostics.
	FilePath string
	// ContextPaths are the context files in the order they were given.
	ContextPaths []string
}

// yakMapping builds the mapping "$yak" answers.
func yakMapping(run Run) *Object {
	paths := make([]*Elem, len(run.ContextPaths))
	for i, p := range run.ContextPaths {
		paths[i] = Item(String(p))
	}
	o := NewObject()
	o.Set("version", false, Done(String(version.Current)))
	o.Set("filepath", false, Done(String(run.FilePath)))
	o.Set("contextpaths", false, Done(NewArray(paths)))
	return o
}

// Document evaluates a single document against the given context value and
// the description of the run behind "$yak". The returned value is fully
// constructed but its leaves are still lazy; use Force to resolve them.
func Document(doc *ast.Document, context Value, run Run) (Value, error) {
	if context == nil {
		context = NewObject()
	}
	ds := &docState{context: context, yak: yakMapping(run)}
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
	case *ast.Yak:
		return env.doc.yak, nil
	case *ast.Field:
		return evalField(node, env)
	case *ast.Index:
		return evalIndex(node, env)
	case *ast.Coalesce:
		return evalCoalesce(node, env)
	case *ast.Unary:
		return evalUnary(node, env)
	case *ast.Binary:
		return evalBinary(node, env)
	case *ast.If:
		return evalIf(node, env)
	case *ast.SeqComp:
		return evalSeqComp(node, env)
	case *ast.MapComp:
		return evalMapComp(node, env)
	case *ast.Local:
		return evalLocal(node, env)
	case *ast.Function:
		return &Function{decl: node, env: env}, nil
	case *ast.Call:
		return evalCall(node, env)
	case *ast.Ident:
		return evalIdent(node, env)
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
	// Bindings sit inside the mapping's own self frame, so a binding may
	// read a field with ".name" just as an entry can.
	child, err := child.pushBindings(node.Binds)
	if err != nil {
		return nil, err
	}

	slots := make([]*Field, len(node.Entries))
	for i, e := range node.Entries {
		slots[i] = obj.reserve(comments(e.Comments), e.Hidden, e.HideNull,
			&Thunk{node: e.Value, env: child, pos: e.Value.Pos()})
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
	items := make([]*Elem, len(node.Items))
	for i, item := range node.Items {
		items[i] = &Elem{
			Comments: comments(item.Comments),
			Value:    &Thunk{node: item.Value, env: env, pos: item.Value.Pos()},
		}
	}
	return NewArray(items), nil
}

// evalSeqComp builds a sequence by evaluating the comprehension's item once
// per item of its source.
func evalSeqComp(node *ast.SeqComp, env *Env) (Value, error) {
	var items []*Elem
	err := iterate(&node.Loop, env, func(child *Env) error {
		items = append(items, &Elem{Value: &Thunk{node: node.Item, env: child, pos: node.Item.Pos()}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return NewArray(items), nil
}

// evalMapComp builds a mapping the same way. Keys have to be resolved as the
// loop runs, because two items that name the same key are a duplicate rather
// than an overwrite, exactly as in a written-out mapping.
func evalMapComp(node *ast.MapComp, env *Env) (Value, error) {
	obj := NewObject()
	e := node.Entry
	err := iterate(&node.Loop, env, func(child *Env) error {
		key, err := evalNode(e.Key, child)
		if err != nil {
			return err
		}
		name, ok := key.(String)
		if !ok {
			return errorf(e.KeyPos, "mapping keys must be strings, found %s", key.TypeName())
		}
		slot := obj.reserve(comments(e.Comments), e.Hidden, e.HideNull,
			&Thunk{node: e.Value, env: child, pos: e.Value.Pos()})
		if err := obj.bind(slot, string(name)); err != nil {
			return errorf(e.KeyPos, "two items of the comprehension produced the key %q", string(name))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// iterate walks the source sequence of a comprehension, calling yield with an
// environment that names the current item. The filter has to be resolved as
// the loop runs, so it forces each item it looks at; the body it guards stays
// lazy like any other value.
func iterate(loop *ast.Loop, env *Env, yield func(*Env) error) error {
	src, err := evalNode(loop.Source, env)
	if err != nil {
		return err
	}
	arr, ok := src.(*Array)
	if !ok {
		return errorf(loop.Source.Pos(), "%q needs a sequence to walk over, found %s", "for", src.TypeName())
	}
	for _, item := range arr.Items() {
		child := env.bindOne(loop.Var, item.Value)
		if loop.Filter != nil {
			keep, err := condition(loop.Filter, child, `the filter of a "for"`)
			if err != nil {
				return err
			}
			if !keep {
				continue
			}
		}
		if err := yield(child); err != nil {
			return err
		}
	}
	return nil
}

// comments carries the comments of a syntax node through to the renderer.
func comments(c ast.Comments) Comments {
	return Comments{Head: c.Head, Line: c.Line, Foot: c.Foot}
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
		return nil, errorf(node.Pos(), "%q reaches past the outermost mapping of the document", node.Src)
	}
	return env.self[node.Up], nil
}

func evalLocal(node *ast.Local, env *Env) (Value, error) {
	child, err := env.pushBindings(node.Binds)
	if err != nil {
		return nil, err
	}
	return evalNode(node.Body, child)
}

// yamlBooleans are the extra boolean spellings YAML accepts and yak does not.
var yamlBooleans = map[string]string{
	"yes": "true", "no": "false",
	"on": "true", "off": "false",
	"y": "true", "n": "false",
}

// evalIdent reads a name back. Bindings come first, so a local named after a
// built-in shadows it.
func evalIdent(node *ast.Ident, env *Env) (Value, error) {
	if t, ok := env.lookup(node.Name); ok {
		return t.Value()
	}
	if b, ok := builtins[node.Name]; ok {
		return b, nil
	}
	if want, ok := yamlBooleans[strings.ToLower(node.Name)]; ok {
		return nil, errorf(node.Pos(),
			"%q is not a boolean in yak; write %s, or quote it to make it a string", node.Name, want)
	}
	names := env.bindingNames()
	if len(names) == 0 {
		return nil, errorf(node.Pos(),
			"unknown identifier %q; strings must be quoted, and no %q binding is in scope here",
			node.Name, "local")
	}
	return nil, errorf(node.Pos(),
		"unknown identifier %q; strings must be quoted; bindings in scope: %s",
		node.Name, quoteNames(names))
}

// evalCoalesce leaves the right hand side unevaluated unless it is needed, so
// that a fallback may itself be an expression that only makes sense when the
// value it replaces is missing.
func evalCoalesce(node *ast.Coalesce, env *Env) (Value, error) {
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	if _, isNull := x.(Null); !isNull {
		return x, nil
	}
	return evalNode(node.Y, env)
}

// evalField reads "x.name". The optional form "x?.name" answers null when
// there is nothing to read, but a value of the wrong type is still an error:
// "?." forgives an absent field, not a misunderstanding about what x is.
func evalField(node *ast.Field, env *Env) (Value, error) {
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	if _, isNull := x.(Null); isNull && node.Optional {
		return Null{}, nil
	}
	obj, ok := x.(*Object)
	if !ok {
		return nil, errorf(node.Pos(), "cannot read field %q from a %s", node.Name, x.TypeName())
	}
	f, ok := obj.Lookup(node.Name)
	if !ok {
		if node.Optional {
			return Null{}, nil
		}
		return nil, errorf(node.Pos(), "no field %q in mapping%s", node.Name, availableFields(obj))
	}
	return f.Value.Value()
}

func evalIndex(node *ast.Index, env *Env) (Value, error) {
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	if _, isNull := x.(Null); isNull && node.Optional {
		return Null{}, nil
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
			if node.Optional {
				return Null{}, nil
			}
			return nil, errorf(node.Index.Pos(), "index %d is out of range for a sequence of length %d", int(n), container.Len())
		}
		return container.Items()[i].Value.Value()
	case *Object:
		name, ok := idx.(String)
		if !ok {
			return nil, errorf(node.Index.Pos(), "mapping index must be a string, found %s", idx.TypeName())
		}
		f, ok := container.Lookup(string(name))
		if !ok {
			if node.Optional {
				return Null{}, nil
			}
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
	return "; available fields: " + quoteNames(names)
}

// quoteNames renders a list of names for a diagnostic, stopping before it
// becomes a wall of text.
func quoteNames(names []string) string {
	if len(names) > 8 {
		names = names[:8]
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = `"` + n + `"`
	}
	return strings.Join(quoted, ", ")
}
