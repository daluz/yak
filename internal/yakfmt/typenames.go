package yakfmt

import (
	"reflect"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// TypeNames says how Source spells a built-in type that has a short name,
// and a conversion named for one.
type TypeNames int

const (
	// KeepTypeNames leaves every name as it was written.
	KeepTypeNames TypeNames = iota
	// ShortTypeNames writes "int" for "integer", "seq" for "sequence" and so
	// on.
	ShortTypeNames
	// LongTypeNames writes "integer" for "int", "sequence" for "seq" and so
	// on.
	LongTypeNames
)

// conversions maps the short name of each conversion built-in to its long
// one.
var conversions = map[string]string{"int": "integer", "bool": "boolean", "str": "string"}

// respell answers the spelling names gives each token that writes a
// built-in type or reads a conversion, keyed by its position, and renames
// the references in tree to match. A conversion is left alone wherever the
// file binds either of its names, since a reference to it may then read
// the binding instead. A name written inside an interpolation is not a
// token of its own and keeps its spelling.
func respell(tree *ast.Stream, toks []token.Token, names TypeNames) map[token.Pos]string {
	if names == KeepTypeNames {
		return nil
	}
	idents := map[token.Pos]string{}
	for _, t := range toks {
		if t.Kind == token.Ident {
			idents[t.Pos] = t.Lit
		}
	}
	var types []token.Pos
	var refs []*ast.Ident
	bound := map[string]bool{}
	visit(reflect.ValueOf(tree), func(n any) {
		switch n := n.(type) {
		case *ast.TypeRef:
			if n.Builtin() {
				types = append(types, n.Pos())
			}
		case *ast.TypeSequence:
			types = append(types, n.Pos())
		case *ast.Ident:
			refs = append(refs, n)
		case *ast.Binding:
			bound[n.Name] = true
		case *ast.Param:
			bound[n.Name] = true
		case *ast.Pattern:
			bound[n.Name] = true
		case *ast.Case:
			bound[n.Name] = true
		case *ast.TypeBind:
			bound[n.Name] = true
		}
	})

	short := map[string]string{}
	for s, long := range ast.TypeAliases {
		short[long] = s
	}
	spell := func(long string) string {
		if s, ok := short[long]; ok && names == ShortTypeNames {
			return s
		}
		return long
	}
	renames := map[token.Pos]string{}
	for _, pos := range types {
		if lit, ok := idents[pos]; ok {
			if long, ok := ast.BuiltinType(lit); ok && spell(long) != lit {
				renames[pos] = spell(long)
			}
		}
	}
	conversion := map[string]string{}
	for s, long := range conversions {
		conversion[s], conversion[long] = long, long
	}
	for _, ref := range refs {
		long, ok := conversion[ref.Name]
		if !ok || bound[long] || bound[short[long]] || idents[ref.Pos()] != ref.Name {
			continue
		}
		if name := spell(long); name != ref.Name {
			renames[ref.Pos()] = name
			ref.Name = name
		}
	}
	return renames
}

// visit calls f with every node that v holds, at any depth.
func visit(v reflect.Value, f func(any)) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			f(v.Interface())
			visit(v.Elem(), f)
		}
	case reflect.Interface:
		if !v.IsNil() {
			visit(v.Elem(), f)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				visit(v.Field(i), f)
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			visit(v.Index(i), f)
		}
	}
}
