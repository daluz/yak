package eval

import (
	"fmt"
	"sort"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
	"github.com/daluz/yak/internal/version"
)

// docState holds the per-document state shared by every environment inside
// that document. A module it imports has a docState of its own, and shares
// its session with the document that imported it.
type docState struct {
	root *Thunk
	// yak is the mapping "$yak" answers.
	yak Value
	// run describes the run, with FilePath naming the file the document was
	// read from.
	run     Run
	session *Session
	// module records a document read by an import rather than rendered.
	module bool
	// schemas are the ones declared at the top of the document, which are
	// what a module offers to the files that import it.
	schemas map[string]*schema
}

// Session is the state a document and every module it imports have in common.
type Session struct {
	run     Run
	context Value
	mods    Modules
	// builtins are the members of the root module. They are found only after
	// every binding in scope, so a let of the same name shadows one rather
	// than colliding with it.
	builtins map[string]*Builtin
	// stack records the positions currently being evaluated so that a cycle
	// can be reported as the chain of references that formed it.
	stack []token.Pos
	// depth counts the function calls and the schema references being
	// evaluated, which bounds a recursion that never ends.
	depth int
	// choosing counts the unions picking an alternative, which they do by
	// checking even when the run skips checks.
	choosing int
}

// skipping reports whether a check should let any value through, filling in
// defaults alone.
func (s *Session) skipping() bool { return s.run.SkipChecks && s.choosing == 0 }

// Modules supplies the modules a document can reach: the root module, whose
// built-ins are bare names, and every module an import names.
type Modules interface {
	Root() []*Builtin
	// Import answers the root of the module node names, which is evaluated
	// in s the first time it is read.
	Import(s *Session, node *ast.Import) (*Thunk, error)
}

// Env is the lexical environment of an expression.
type Env struct {
	doc *docState
	// self holds the enclosing mappings, innermost first. Sequences do not
	// add a level, so ".." always names the nearest enclosing mapping.
	self []*Object
	// scope is the innermost frame of let bindings, or nil at the top of
	// a document.
	scope *scope
}

// scope is one frame of let bindings, or of the schemas a block declares.
// Frames link outwards, so an inner binding shadows an outer one of the same
// name.
type scope struct {
	outer *scope
	binds map[string]*Thunk
	// schemas are looked up apart from binds, by tags and by types.
	schemas map[string]*schema
}

// atRoot reports whether e is the environment a document's body is
// evaluated in, before the body adds anything to it.
func (e *Env) atRoot() bool { return len(e.self) == 0 && e.scope == nil }

func (e *Env) pushSelf(o *Object) *Env {
	self := make([]*Object, 0, len(e.self)+1)
	self = append(self, o)
	self = append(self, e.self...)
	return &Env{doc: e.doc, self: self, scope: e.scope}
}

// regions holds the environments a block's bindings create. A name may be
// bound more than once in a block, so what a reference reads depends on where
// it is written: envs[j] is in effect below the j'th binding, and envs[0]
// above them all.
type regions struct {
	envs []*Env
	// decl holds the position of each binding, so decl[j] opens envs[j+1].
	// It is empty when one environment covers the whole block.
	decl []token.Pos
	// schemas are the ones the block declares, or nil.
	schemas map[string]*schema
}

// at returns the environment a node written at pos reads names in, which is
// the one opened by the last binding declared above it.
func (r *regions) at(pos token.Pos) *Env {
	j := 0
	for j < len(r.decl) && before(r.decl[j], pos) {
		j++
	}
	return r.envs[j]
}

// last returns the environment below every binding of the block.
func (r *regions) last() *Env { return r.envs[len(r.envs)-1] }

// before reports whether a is written above b, or to its left on one line.
func before(a, b token.Pos) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Col < b.Col
}

// pushBindings returns the environments that binds create over e. A binding
// is evaluated in the environment above its own line, so a rebinding reads
// the binding it replaces while a lone one reads itself; a binding that ends
// up needing itself is reported as a cycle.
//
// The schemas among binds share one frame beneath the bindings, so a
// binding can apply them, and each is read in the region it is declared in,
// so it can name the bindings around it.
func (e *Env) pushBindings(binds []*ast.Binding) *regions {
	var values []*ast.Binding
	var decls []*ast.Binding
	for _, b := range binds {
		if _, ok := b.Value.(*ast.Schema); ok {
			decls = append(decls, b)
		} else {
			values = append(values, b)
		}
	}
	if len(decls) == 0 {
		return e.pushValues(values)
	}
	schemas := make(map[string]*schema, len(decls))
	r := (&Env{doc: e.doc, self: e.self, scope: &scope{outer: e.scope, schemas: schemas}}).pushValues(values)
	for _, b := range decls {
		schemas[b.Name] = &schema{name: b.Name, decl: b.Value.(*ast.Schema), env: r.at(b.Pos())}
	}
	r.schemas = schemas
	return r
}

func (e *Env) pushValues(binds []*ast.Binding) *regions {
	if len(binds) == 0 {
		return &regions{envs: []*Env{e}}
	}
	// The outermost frame holds the first binding of every name, which is
	// what a reference written above them all reads. It is filled in as the
	// bindings are made, which is before anything can read it, so each
	// binding can name the ones around it, including later ones.
	first := map[string]*Thunk{}
	env := &Env{doc: e.doc, self: e.self, scope: &scope{outer: e.scope, binds: first}}
	// With no name bound twice, that one frame covers the whole block and
	// nothing has to care where it is read from.
	if !rebinds(binds) {
		for _, b := range binds {
			bind(b, env, first)
		}
		return &regions{envs: []*Env{env}}
	}
	r := &regions{envs: make([]*Env, 1, len(binds)+1), decl: make([]token.Pos, len(binds))}
	r.envs[0] = env
	for i, b := range binds {
		frame := map[string]*Thunk{}
		bind(b, env, frame)
		for name, t := range frame {
			if _, ok := first[name]; !ok {
				first[name] = t
			}
		}
		env = &Env{doc: e.doc, self: e.self, scope: &scope{outer: env.scope, binds: frame}}
		r.decl[i] = b.Pos()
		r.envs = append(r.envs, env)
	}
	return r
}

// bindPattern returns an environment that adds a frame holding the names pat
// binds in the value of t, which is how a comprehension names the item it is
// looking at. fit has to have checked the value against pat already.
func (e *Env) bindPattern(pat *ast.Pattern, t *Thunk) *Env {
	binds := map[string]*Thunk{}
	pat.Walk(func(name *ast.Pattern, path []int) {
		binds[name.Name] = part(t, path)
	})
	return &Env{doc: e.doc, self: e.self, scope: &scope{outer: e.scope, binds: binds}}
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

// Run describes the rendering itself, which the language reads as "$yak",
// "$filepath" and "$workdir".
type Run struct {
	// FilePath is the template as the command line named it. Input read
	// from standard input is called "<stdin>", as it is in diagnostics.
	FilePath string
	// Workdir is the absolute path of the directory yak was invoked from.
	Workdir string
	// Format is the canonical name of the output format, never an alias.
	Format string
	// NonStrict turns strict mode off, admitting the built-in modules whose
	// answers can change from one rendering to the next. It is negative so
	// that the zero Run is strict.
	NonStrict bool
	// SkipChecks lets every value through a schema, which then only fills
	// in defaults.
	SkipChecks bool
}

// Strict reports whether the run is in strict mode, which is what
// "$yak.strict" says.
func (r Run) Strict() bool { return !r.NonStrict }

// yakMapping builds the mapping "$yak" answers.
func yakMapping(run Run) *Object {
	o := NewObject()
	o.Set("version", false, Done(String(version.Current)))
	o.Set("format", false, Done(String(run.Format)))
	o.Set("strict", false, Done(Bool(run.Strict())))
	return o
}

// Document evaluates a single document against the given context value and
// the description of the run behind "$yak", reaching modules through mods.
// The returned value is fully constructed but its leaves are still lazy; use
// Force to resolve them.
func Document(doc *ast.Document, context Value, run Run, mods Modules) (Value, error) {
	if context == nil {
		context = NewObject()
	}
	return newSession(context, run, mods).document(doc, run.FilePath, false).Value()
}

func newSession(context Value, run Run, mods Modules) *Session {
	s := &Session{run: run, context: context, mods: mods, builtins: map[string]*Builtin{}}
	for _, b := range mods.Root() {
		s.builtins[b.Name] = b
	}
	return s
}

// Document answers the root of doc, a module read from file, which is
// evaluated in the session the first time it is read. "$yak" describes the
// run as it does for the template, except that it names file as the one
// being read.
func (s *Session) Document(doc *ast.Document, file string) *Thunk {
	return s.document(doc, file, true)
}

func (s *Session) document(doc *ast.Document, file string, module bool) *Thunk {
	run := s.run
	run.FilePath = file
	ds := &docState{yak: yakMapping(run), run: run, session: s, module: module}
	ds.root = &Thunk{node: doc.Body, env: &Env{doc: ds}, pos: doc.Pos()}
	return ds.root
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
		return env.doc.session.context, nil
	case *ast.Yak:
		return env.doc.yak, nil
	case *ast.FilePath:
		return String(env.doc.run.FilePath), nil
	case *ast.Workdir:
		return String(env.doc.run.Workdir), nil
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
	case *ast.Match:
		return evalMatch(node, env)
	case *ast.SeqComp:
		return evalSeqComp(node, env)
	case *ast.MapComp:
		return evalMapComp(node, env)
	case *ast.Let:
		return evalLet(node, env)
	case *ast.Import:
		return evalImport(node, env)
	case *ast.Tag:
		return evalTag(node, env)
	case *ast.Function:
		return &function{decl: node, env: env}, nil
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
// matter which pass names them. A "<<" folds in after both passes, once every
// key the mapping writes for itself is known.
func evalMapping(node *ast.Mapping, env *Env) (Value, error) {
	obj := NewObject()
	// Bindings sit inside the mapping's own self frame, so a binding may
	// read a field with ".name" just as an entry can. An entry reads them
	// as of its own line, which is all a rebinding changes.
	binds := env.pushSelf(obj).pushBindings(node.Binds)
	root := env.atRoot()
	if root {
		if err := env.doc.declare(binds.schemas); err != nil {
			return nil, err
		}
	}

	slots := make([]int, len(node.Entries))
	for i, e := range node.Entries {
		// A merge holds no entry of its own, so it reserves nothing; it
		// folds into the mapping once every key in it is known.
		if e.Merge {
			continue
		}
		child := binds.at(e.KeyPos)
		slots[i] = obj.reserve(carried(comments(e.Comments), e.Value, child), e.Hidden, e.HideNull,
			&Thunk{node: e.Value, env: child, pos: e.Value.Pos()})
	}
	for i, e := range node.Entries {
		if name, ok := literalKey(e); ok {
			if err := obj.bind(slots[i], name); err != nil {
				return nil, errorAt(e.KeyPos, err)
			}
		}
	}
	for i, e := range node.Entries {
		if e.Merge {
			continue
		}
		if _, ok := literalKey(e); ok {
			continue
		}
		key, err := evalNode(e.Key, binds.at(e.KeyPos))
		if err != nil {
			return nil, err
		}
		name, ok := key.(String)
		if !ok {
			return nil, errorf(e.KeyPos, "mapping keys must be strings, found %s", key.TypeName())
		}
		if err := obj.bind(slots[i], string(name)); err != nil {
			return nil, errorAt(e.KeyPos, err)
		}
	}
	// Merges come last, so that a key the mapping writes itself is known
	// however it was written, and a merged key can be told from one it
	// wrote. Each one folds in at the position it was written at, which is
	// the number of entries above it plus whatever earlier merges added.
	written, added := 0, 0
	for _, e := range node.Entries {
		if !e.Merge {
			written++
			continue
		}
		n, err := applyMerge(obj, written+added, e, binds.at(e.KeyPos))
		if err != nil {
			return nil, err
		}
		added += n
	}
	if s := binds.schemas[ast.SchemaRoot]; root && s != nil {
		return s.apply(obj, node.Pos(), true)
	}
	return obj, nil
}

// applyMerge folds the mapping a "<<" entry names into the one being built,
// at the position "at" the entry was written at. What the merge brings in
// overrides the entries above it and is overridden by the ones below, so a
// mapping reads top to bottom however its entries got there. It answers how
// many entries the merge added.
func applyMerge(o *Object, at int, e *ast.Entry, env *Env) (int, error) {
	v, err := evalNode(e.Value, env)
	if err != nil {
		return 0, err
	}
	// ":?" makes the merge itself optional, which is what reading an
	// override out of the context with "$$?.name" needs.
	if _, null := v.(Null); null && e.HideNull {
		return 0, nil
	}
	src, ok := v.(*Object)
	if !ok {
		return 0, errorf(e.KeyPos, "%q needs a mapping to merge, found %s", "<<", v.TypeName())
	}
	c := carried(comments(e.Comments), e.Value, env)
	added, placed := 0, false
	for _, f := range src.Fields() {
		q, held := o.index[f.Name]
		var target *Field
		switch {
		case !held:
			brought := *f
			brought.Hidden = brought.Hidden || e.Hidden
			o.insertAt(at+added, &brought)
			added++
			target = &brought
		case q < at+added:
			// The merge overrides an entry written above it. That entry
			// keeps the place and the comments it was written with; the
			// merge decides its value and whether it is hidden.
			value, err := combined(o.fields[q], f, true, false)
			if err != nil {
				return 0, err
			}
			target = o.fields[q]
			target.Value, target.Hidden, target.HideNull = value, f.Hidden || e.Hidden, f.HideNull
		default:
			// The entry below was written after the merge, so it wins,
			// and keeps everything it says about itself.
			value, err := combined(f, o.fields[q], true, false)
			if err != nil {
				return 0, err
			}
			target = o.fields[q]
			target.Value = value
		}
		if !placed {
			// The comments written on the "<<" belong to the first entry
			// it decides, ahead of whatever that entry carried in.
			target.Head = append(append([]string(nil), c.Head...), target.Head...)
			if target.Line == "" {
				target.Line = c.Line
			}
			placed = true
		}
	}
	return added, nil
}

// literalKey returns the name of a key that is known without evaluation.
func literalKey(e *ast.Entry) (string, bool) {
	if e.Merge || e.Computed {
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
			Comments: carried(comments(item.Comments), item.Value, env),
			Value:    &Thunk{node: item.Value, env: env, pos: item.Value.Pos()},
		}
	}
	return NewArray(items), nil
}

// evalSeqComp builds a sequence by evaluating the comprehension's item once
// per item of its source.
func evalSeqComp(node *ast.SeqComp, env *Env) (Value, error) {
	var items []*Elem
	err := iterate(node.Loops, env, func(child *Env) error {
		items = append(items, &Elem{
			Comments: carried(Comments{}, node.Item, child),
			Value:    &Thunk{node: node.Item, env: child, pos: node.Item.Pos()},
		})
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
	err := iterate(node.Loops, env, func(child *Env) error {
		key, err := evalNode(e.Key, child)
		if err != nil {
			return err
		}
		name, ok := key.(String)
		if !ok {
			return errorf(e.KeyPos, "mapping keys must be strings, found %s", key.TypeName())
		}
		slot := obj.reserve(carried(comments(e.Comments), e.Value, child), e.Hidden, e.HideNull,
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

// iterate walks the "for" clauses of a comprehension, calling yield with an
// environment that names the current item of each. A later clause is
// evaluated once per item of those before it, and can read their names.
func iterate(loops []ast.Loop, env *Env, yield func(*Env) error) error {
	if len(loops) == 0 {
		return yield(env)
	}
	return walk(&loops[0], env, func(child *Env) error {
		return iterate(loops[1:], child, yield)
	})
}

// walk walks the source sequence of one "for" clause, calling yield with an
// environment that names the current item. The filter has to be resolved as
// the loop runs, so it forces each item it looks at; the body it guards stays
// lazy like any other value. An item the pattern destructures is checked as
// it is reached, whether or not anything reads the names it binds.
func walk(loop *ast.Loop, env *Env, yield func(*Env) error) error {
	src, err := evalNode(loop.Source, env)
	if err != nil {
		return err
	}
	arr, ok := src.(*Array)
	if !ok {
		return errorf(loop.Source.Pos(), "%q needs a sequence to walk over, found %s", "for", src.TypeName())
	}
	for i, item := range arr.Items() {
		if err := fit(loop.Pattern, item.Value, fmt.Sprintf(" (the item at index %d of the source)", i)); err != nil {
			return err
		}
		child := env.bindPattern(loop.Pattern, item.Value)
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

// carried adds to c the comments written on the binding that node names. A
// binding renders nothing of its own, so its comments come out on whatever
// reads it back, below the comments that place was written with and beside
// it only when it has no trailing comment of its own.
func carried(c Comments, node ast.Node, env *Env) Comments {
	id, ok := node.(*ast.Ident)
	if !ok {
		return c
	}
	t, ok := env.lookup(id.Name)
	if !ok || (len(t.comments.Head) == 0 && t.comments.Line == "") {
		return c
	}
	c.Head = append(append([]string(nil), c.Head...), t.comments.Head...)
	if c.Line == "" {
		c.Line = t.comments.Line
	}
	return c
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
			return nil, errorAt(part.Expr.Pos(), err)
		}
		sb.WriteString(s)
	}
	return String(sb.String()), nil
}

func evalSelf(node *ast.Self, env *Env) (Value, error) {
	if node.Up >= len(env.self) {
		return nil, errorf(node.Pos(), "%q reaches past the outermost mapping of the document", strings.Repeat(".", node.Up+1))
	}
	return env.self[node.Up], nil
}

// evalLet evaluates a body that is not a mapping. The body sits below every
// binding of the block, so it reads the last of each.
func evalLet(node *ast.Let, env *Env) (Value, error) {
	binds := env.pushBindings(node.Binds)
	root := env.atRoot()
	if root {
		if err := env.doc.declare(binds.schemas); err != nil {
			return nil, err
		}
	}
	v, err := evalNode(node.Body, binds.last())
	if err != nil {
		return nil, err
	}
	if s := binds.schemas[ast.SchemaRoot]; root && s != nil {
		_, fresh := node.Body.(*ast.Mapping)
		return s.apply(v, node.Body.Pos(), fresh)
	}
	return v, nil
}

// yamlBooleans are the extra boolean spellings YAML accepts and yak does not.
var yamlBooleans = map[string]string{
	"yes": "true", "no": "false",
	"on": "true", "off": "false",
	"y": "true", "n": "false",
}

// evalIdent reads a name back. Bindings come first, so a let named after a
// built-in shadows it.
func evalIdent(node *ast.Ident, env *Env) (Value, error) {
	if t, ok := env.lookup(node.Name); ok {
		return t.Value()
	}
	if b, ok := env.doc.session.builtins[node.Name]; ok {
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
			node.Name, "let")
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
	return field(obj, node.Name, node.Optional, node.Pos())
}

// field reads the field name of o, answering null for an absent one when the
// access is optional.
func field(o *Object, name string, optional bool, pos token.Pos) (Value, error) {
	f, ok := o.Lookup(name)
	if !ok {
		if optional {
			return Null{}, nil
		}
		return nil, errorf(pos, "no field %q in mapping%s", name, availableFields(o))
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
		return field(container, string(name), node.Optional, node.Index.Pos())
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
