package eval

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// Schema is a type a "schema" statement declares, closed over the block it
// was declared in: the names its types, defaults and checks read are the
// ones in scope there.
type Schema struct {
	// name is the schema's name, or ast.SchemaRoot or ast.SchemaContext.
	name string
	decl *ast.Schema
	env  *Env
}

// label names the schema in a diagnostic.
func (s *Schema) label() string {
	switch s.name {
	case ast.SchemaRoot:
		return "the document"
	case ast.SchemaContext:
		return "the context"
	}
	return fmt.Sprintf("schema %q", s.name)
}

// declare records the schemas a document declares at its top, and checks
// the context against the one it declares for it. The context is plain data,
// so it is checked in full before anything reads it, and every reader sees
// it with its defaults filled in.
func (d *docState) declare(schemas map[string]*Schema) error {
	d.schemas = schemas
	s := schemas[ast.SchemaContext]
	if s == nil {
		return nil
	}
	if d.module {
		return errorf(s.decl.Pos(), "a module cannot declare a schema for the context; only the template being rendered can")
	}
	v, err := s.apply(d.session.context, s.decl.Pos(), false)
	if err != nil {
		return err
	}
	if err := Force(v); err != nil {
		return annotate(err, s.decl.Pos())
	}
	d.session.context = v
	return nil
}

// evalTag checks the value a tag is written on against the schema it names.
// A mapping written right after the tag is the value checked, so its own
// entries read the defaults it is given; any other mapping is copied first,
// leaving the one it was read from as it was.
func evalTag(node *ast.Tag, env *Env) (Value, error) {
	s, err := env.findSchema(node.Module, node.Name, node.Pos(), true)
	if err != nil {
		return nil, err
	}
	v, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	_, fresh := node.X.(*ast.Mapping)
	return s.apply(v, node.X.Pos(), fresh)
}

// apply checks v, written at pos, against the schema. fresh says that v is a
// mapping nothing else holds yet, which then takes the defaults itself.
func (s *Schema) apply(v Value, pos token.Pos, fresh bool) (Value, error) {
	return s.check(v, s.decl.Type, site{of: s.label(), pos: pos}, fresh)
}

// findSchema answers the schema a tag or a type names. A module's schemas
// are the ones declared at the top of its document. tag says that a tag
// names it, where a "!" may have been meant as a negation.
func (e *Env) findSchema(module, name string, pos token.Pos, tag bool) (*Schema, error) {
	if module != "" {
		return e.moduleSchema(module, name, pos)
	}
	for sc := e.scope; sc != nil; sc = sc.outer {
		if s, ok := sc.schemas[name]; ok {
			return s, nil
		}
	}
	if t, ok := e.lookup(name); ok {
		if _, fn := t.node.(*ast.Function); fn {
			return nil, errorf(pos, "%q is a function, not a schema; call it as %s(...)", name, name)
		}
		return nil, errorf(pos, "%q is a binding, not a schema", name)
	}
	if _, ok := e.doc.session.builtins[name]; ok {
		return nil, errorf(pos, "%q is a function, not a schema; call it as %s(...)", name, name)
	}
	if tag {
		return nil, errorf(pos, "no schema %q is in scope; to negate a value, %q is written %q", name, "!", token.KeywordNot)
	}
	return nil, errorf(pos, "no schema or type %q is in scope", name)
}

// moduleSchema answers the schema name that the module imported as module
// declares. Reading it loads the module.
func (e *Env) moduleSchema(module, name string, pos token.Pos) (*Schema, error) {
	t, ok := e.lookup(module)
	var imp *ast.Import
	if ok {
		imp, ok = t.node.(*ast.Import)
	}
	if !ok {
		return nil, errorf(pos, "%q is not an imported module, so %q names no schema", module, module+"."+name)
	}
	s := e.doc.session
	root, err := s.mods.Import(s, imp)
	if err != nil {
		return nil, err
	}
	if _, err := root.Value(); err != nil {
		return nil, err
	}
	var schemas map[string]*Schema
	if root.env != nil {
		schemas = root.env.doc.schemas
	}
	if sc, ok := schemas[name]; ok && sc.name != ast.SchemaRoot && sc.name != ast.SchemaContext {
		return sc, nil
	}
	return nil, errorf(pos, "module %q declares no schema %q", strings.Join(imp.Path, "."), name)
}

// site says which value a check is about: the schema or the document it
// belongs to, the path to it from there, and where it was written.
type site struct {
	of, path string
	pos      token.Pos
}

func (w site) String() string {
	switch {
	case w.path == "":
		return w.of
	case strings.HasPrefix(w.path, "["):
		return fmt.Sprintf("item %s of %s", w.path, w.of)
	}
	return fmt.Sprintf("field %q of %s", w.path, w.of)
}

// field is the site of the field name, written at pos when that is known.
func (w site) field(name string, pos token.Pos) site {
	path := name
	if w.path != "" {
		path = w.path + "." + name
	}
	return site{of: w.of, path: path, pos: w.at(pos)}
}

// item is the site of the item at index i.
func (w site) item(i int, pos token.Pos) site {
	return site{of: w.of, path: fmt.Sprintf("%s[%d]", w.path, i), pos: w.at(pos)}
}

// at answers pos, or where the enclosing value was written when pos is not
// known, as it is not for what a context file holds.
func (w site) at(pos token.Pos) token.Pos {
	if pos.IsValid() {
		return pos
	}
	return w.pos
}

// How a value failed a check, which is what a union decides by: a value of
// the wrong kind fails outright, while a mapping that is missing a field was
// at least meant for the alternative that says so.
const (
	kindMisfit = 1
	deepMisfit = 2
)

func misfit(level int, pos token.Pos, format string, args ...any) *Error {
	e := errorf(pos, format, args...)
	e.misfit = level
	return e
}

func mismatch(at site, want string, v Value) *Error {
	return misfit(kindMisfit, at.pos, "%s expects %s, found %s", at, want, found(v))
}

// check checks v against the type t, which s declares, and answers v with
// the defaults its mappings declare filled in. The items of a sequence and
// the fields of a mapping are each checked when they are first read.
func (s *Schema) check(v Value, t ast.Node, at site, fresh bool) (Value, error) {
	switch t := t.(type) {
	case *ast.TypeRef:
		if t.Module == "" && slices.Contains(ast.BuiltinTypes, t.Name) {
			if t.Name == "any" || v.TypeName() == t.Name {
				return v, nil
			}
			return nil, mismatch(at, t.Name, v)
		}
		target, err := s.env.findSchema(t.Module, t.Name, t.Pos(), false)
		if err != nil {
			return nil, err
		}
		session := s.env.doc.session
		if session.depth >= maxCallDepth {
			return nil, errorf(t.Pos(), "%s refers to itself without ever reaching a type", target.label())
		}
		session.depth++
		defer func() { session.depth-- }()
		return target.check(v, target.decl.Type, at, fresh)
	case *ast.TypeLiteral:
		want, err := evalNode(t.Value, s.env)
		if err != nil {
			return nil, err
		}
		if requireScalar(v) == nil {
			if eq, _ := Equal(v, want); eq {
				return v, nil
			}
		}
		return nil, mismatch(at, describe(t), v)
	case *ast.TypeSequence:
		arr, ok := v.(*Array)
		if !ok {
			return nil, mismatch(at, "sequence", v)
		}
		items := make([]*Elem, arr.Len())
		for i, e := range arr.Items() {
			items[i] = &Elem{Comments: e.Comments, Value: s.lazy(e.Value, t.Item, at.item(i, e.Value.pos), false)}
		}
		return NewArray(items), nil
	case *ast.TypeMapping:
		return s.checkMapping(v, t, at, fresh)
	case *ast.TypeUnion:
		var deep error
		for _, alt := range t.Alts {
			r, err := s.check(v, alt, at, fresh)
			if err == nil {
				return r, nil
			}
			if e, ok := err.(*Error); !ok || e.misfit == 0 {
				return nil, err
			} else if e.misfit == deepMisfit && deep == nil {
				deep = err
			}
		}
		if deep != nil {
			return nil, deep
		}
		return nil, mismatch(at, describe(t), v)
	case *ast.TypeCheck:
		r, err := s.check(v, t.Type, at, fresh)
		if err != nil {
			return nil, err
		}
		pred, err := evalNode(t.Pred, s.env)
		if err != nil {
			return nil, err
		}
		answer, err := Call(pred, t.Pred.Pos(), r)
		if err != nil {
			return nil, err
		}
		ok, isBool := answer.(Bool)
		if !isBool {
			return nil, errorf(t.Pred.Pos(), "the check of %s must answer a boolean, found %s", at, answer.TypeName())
		}
		if !ok {
			return nil, misfit(deepMisfit, at.pos, "%s is rejected by the check at %d:%d",
				at, t.Pred.Pos().Line, t.Pred.Pos().Col)
		}
		return r, nil
	}
	return nil, errorf(t.Pos(), "internal error: cannot check against %T", t)
}

// lazy answers a thunk that checks the value of t against typ when it is
// first read. An optional field may be null whatever its type.
func (s *Schema) lazy(t *Thunk, typ ast.Node, at site, optional bool) *Thunk {
	return &Thunk{env: s.env, pos: at.pos, fn: func() (Value, error) {
		v, err := t.Value()
		if err != nil {
			return nil, err
		}
		if _, null := v.(Null); null && optional {
			return v, nil
		}
		return s.check(v, typ, at, false)
	}}
}

// typeField is one field of a mapping type, along with the schema that wrote
// it, whose scope its type and default are read in.
type typeField struct {
	owner *Schema
	decl  *ast.TypeMember
}

// checkMapping checks the shape of a mapping: every field it needs is there,
// and every key it holds is one the type declares, unless the schema is
// open. A field written with ":?" whose value is null counts as left out,
// and a hidden one is not checked at all. The values are checked when they
// are read, and a default is evaluated with "." naming the checked mapping.
func (s *Schema) checkMapping(v Value, t *ast.TypeMapping, at site, fresh bool) (Value, error) {
	obj, ok := v.(*Object)
	if !ok {
		return nil, mismatch(at, "mapping", v)
	}
	fields, keys, err := s.members(t, map[*ast.TypeMapping]bool{})
	if err != nil {
		return nil, err
	}
	declared := make(map[string]typeField, len(fields))
	names := make([]string, len(fields))
	for i, m := range fields {
		declared[m.decl.Name] = m
		names[i] = m.decl.Name
	}

	// check records the type each field of obj is checked against, by its
	// position, and defaulted the fields that take a default instead.
	check := map[int]typeField{}
	defaulted := map[int]typeField{}
	present := map[string]bool{}
	for i, f := range obj.Fields() {
		if f.Hidden {
			continue
		}
		m, isDeclared := declared[f.Name]
		if f.HideNull {
			fv, err := f.Value.Value()
			if err != nil {
				return nil, err
			}
			if _, null := fv.(Null); null {
				if isDeclared && m.decl.Default != nil {
					present[f.Name] = true
					defaulted[i] = m
				}
				continue
			}
		}
		fieldAt := at.field(f.Name, f.Value.pos)
		switch {
		case isDeclared:
			present[f.Name] = true
			check[i] = m
		case keys != nil:
			if _, err := keys.owner.check(String(f.Name), keys.decl.Key, fieldAt, false); err != nil {
				if e, ok := err.(*Error); !ok || e.misfit == 0 {
					return nil, err
				}
				return nil, misfit(deepMisfit, fieldAt.pos, "%s has a key %q, which is not %s",
					at, f.Name, describe(keys.decl.Key))
			}
			check[i] = *keys
		case !s.decl.Open:
			return nil, misfit(deepMisfit, fieldAt.pos, "%s has no field %q%s", at, f.Name, declaredFields(names))
		}
	}
	for _, m := range fields {
		if !present[m.decl.Name] && m.decl.Default == nil && !m.decl.Optional {
			return nil, misfit(deepMisfit, at.pos, "%s needs a field %q", at, m.decl.Name)
		}
	}

	target := obj
	if !fresh {
		target = NewObject()
		for _, f := range obj.Fields() {
			target.Add(f)
		}
	}
	for i, m := range check {
		f := target.fields[i]
		f.Value = m.owner.lazy(f.Value, m.decl.Type, at.field(f.Name, f.Value.pos), m.decl.Optional)
	}
	for i, m := range defaulted {
		f := target.fields[i]
		f.Value = m.owner.defaulted(target, m, at.field(f.Name, f.Value.pos))
	}
	for _, m := range fields {
		if !present[m.decl.Name] && m.decl.Default != nil {
			target.Set(m.decl.Name, false, m.owner.defaulted(target, m, at.field(m.decl.Name, m.decl.Default.Pos())))
		}
	}
	return target, nil
}

// defaulted answers the default of the field m declares, checked against
// its type like a value that was written.
func (s *Schema) defaulted(target *Object, m typeField, at site) *Thunk {
	def := &Thunk{node: m.decl.Default, env: s.env.pushSelf(target), pos: m.decl.Default.Pos()}
	return s.lazy(def, m.decl.Type, at, m.decl.Optional)
}

// declaredFields lists the fields a mapping type declares, for a diagnostic
// about one it does not.
func declaredFields(names []string) string {
	if len(names) == 0 {
		return " (it declares none)"
	}
	return "; it declares " + quoteNames(names)
}

// members answers the fields of t in order, with the ones a "<<" merges in
// taking the place of the merge, and a field written below overriding one
// of the same name above it, as in a mapping. It answers the "(K): V" member
// that types the other keys too, or nil.
func (s *Schema) members(t *ast.TypeMapping, seen map[*ast.TypeMapping]bool) ([]typeField, *typeField, error) {
	if seen[t] {
		return nil, nil, errorf(t.Pos(), "this mapping type merges itself")
	}
	seen[t] = true
	defer delete(seen, t)
	var out []typeField
	var keys *typeField
	index := map[string]int{}
	add := func(m typeField) {
		if i, ok := index[m.decl.Name]; ok {
			out[i] = m
			return
		}
		index[m.decl.Name] = len(out)
		out = append(out, m)
	}
	for _, decl := range t.Members {
		switch {
		case decl.Merge:
			owner, mt, err := s.mappingType(decl.Type)
			if err != nil {
				return nil, nil, err
			}
			merged, mergedKeys, err := owner.members(mt, seen)
			if err != nil {
				return nil, nil, err
			}
			for _, m := range merged {
				add(m)
			}
			if mergedKeys != nil {
				keys = mergedKeys
			}
		case decl.Key != nil:
			keys = &typeField{owner: s, decl: decl}
		default:
			add(typeField{owner: s, decl: decl})
		}
	}
	return out, keys, nil
}

// mappingType answers the mapping type that t is or names, and the schema
// whose scope it is read in, for a "<<" to merge.
func (s *Schema) mappingType(t ast.Node) (*Schema, *ast.TypeMapping, error) {
	for hops := 0; hops < maxCallDepth; hops++ {
		switch tt := t.(type) {
		case *ast.TypeMapping:
			return s, tt, nil
		case *ast.TypeRef:
			if tt.Module != "" || !slices.Contains(ast.BuiltinTypes, tt.Name) {
				target, err := s.env.findSchema(tt.Module, tt.Name, tt.Pos(), false)
				if err != nil {
					return nil, nil, err
				}
				s, t = target, target.decl.Type
				continue
			}
		}
		return nil, nil, errorf(t.Pos(), "%q merges the fields of a mapping type, found %s", "<<", describe(t))
	}
	return nil, nil, errorf(t.Pos(), "%s refers to itself without ever reaching a type", s.label())
}

// describe writes a type the way a diagnostic names it.
func describe(t ast.Node) string {
	switch t := t.(type) {
	case *ast.TypeRef:
		if t.Module == "" && slices.Contains(ast.BuiltinTypes, t.Name) {
			return t.Name
		}
		if t.Module != "" {
			return fmt.Sprintf("schema %q", t.Module+"."+t.Name)
		}
		return fmt.Sprintf("schema %q", t.Name)
	case *ast.TypeLiteral:
		switch v := t.Value.(type) {
		case *ast.String:
			return strconv.Quote(v.Literal())
		case *ast.Int:
			return strconv.FormatInt(v.Value, 10)
		case *ast.Float:
			return strconv.FormatFloat(v.Value, 'g', -1, 64)
		case *ast.Bool:
			return strconv.FormatBool(v.Value)
		}
		return "null"
	case *ast.TypeSequence:
		return "sequence"
	case *ast.TypeMapping:
		return "mapping"
	case *ast.TypeUnion:
		alts := make([]string, len(t.Alts))
		for i, alt := range t.Alts {
			alts[i] = describe(alt)
		}
		return strings.Join(alts, " or ")
	case *ast.TypeCheck:
		return describe(t.Type)
	}
	return "a type"
}

// found writes a value for a diagnostic about it: a scalar as itself, and
// anything else by its type.
func found(v Value) string {
	if s, ok := v.(String); ok {
		return strconv.Quote(string(s))
	}
	if requireScalar(v) == nil {
		text, _ := stringify(v)
		return text
	}
	return v.TypeName()
}
