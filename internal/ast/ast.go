// Package ast defines the syntax tree produced by the yak parser.
package ast

import "github.com/daluz/yak/internal/token"

// Node is any syntax tree node.
type Node interface {
	Pos() token.Pos
	node()
}

// Base carries the source position shared by every node.
type Base struct{ P token.Pos }

// Pos returns the position of the first token of the node.
func (b Base) Pos() token.Pos { return b.P }

func (Base) node() {}

// At builds a Base for the given position.
func At(p token.Pos) Base { return Base{P: p} }

// Stream is a parsed file: one or more documents separated by "---".
type Stream struct {
	File string
	Docs []*Document
}

// Document is a single YAML document within a stream.
type Document struct {
	Base
	Body Node
}

// Mapping is a collection of key/value entries, written either in block form
// or in flow form with braces.
type Mapping struct {
	Base
	// Binds holds the "local" bindings written among the entries. They cover
	// the whole mapping, wherever in it they appear, and they can see the
	// mapping itself, so a binding may refer to a field with ".name".
	Binds   []*Binding
	Entries []*Entry
	Flow    bool
}

// Binding is one "name = value" pair introduced by "local".
type Binding struct {
	Base
	Name  string
	Value Node
}

// Local scopes bindings over a body that is not a mapping. Bindings written
// in a mapping belong to the Mapping instead, so that they can refer to its
// fields.
type Local struct {
	Base
	Binds []*Binding
	Body  Node
}

// Comments are the source comments attached to a node that is rendered. The
// renderer writes them back out around it.
type Comments struct {
	// Head holds the whole-line comments written above the node, in source
	// order. Each keeps the "#" it was written with.
	Head []string
	// Line holds a comment written after the node on the node's own line.
	Line string
	// Foot holds the comments left over at the end of a document, which the
	// last node of that document carries.
	Foot []string
}

// Entry is one key/value pair of a Mapping.
type Entry struct {
	Comments
	// Key is a *String for literal keys (bare identifiers are normalized into
	// strings) or an arbitrary expression for computed keys written as [expr].
	Key Node
	// Computed records that the key was written in bracket form.
	Computed bool
	// Hidden records that the entry was written with "::" and must be omitted
	// from rendered output while remaining visible to references.
	Hidden bool
	// HideNull records that the entry was written with "::?", which omits it
	// from rendered output only when its value turns out to be null.
	HideNull bool
	Value    Node
	KeyPos   token.Pos
}

// Sequence is an ordered list of items.
type Sequence struct {
	Base
	Items []*Item
	Flow  bool
}

// Item is one element of a Sequence. It wraps the element's value so that the
// element can carry comments of its own, as a mapping entry does.
type Item struct {
	Comments
	Value Node
}

// StringPart is one piece of a string literal.
type StringPart struct {
	// Text is the literal text when Expr is nil.
	Text string
	// Expr is the parsed interpolation when non-nil.
	Expr Node
}

// String is a string literal, possibly containing interpolations.
type String struct {
	Base
	Parts []StringPart
}

// IsLiteral reports whether the string has no interpolations.
func (s *String) IsLiteral() bool {
	return len(s.Parts) == 0 || (len(s.Parts) == 1 && s.Parts[0].Expr == nil)
}

// Literal returns the constant text of a string without interpolations.
func (s *String) Literal() string {
	if len(s.Parts) == 0 {
		return ""
	}
	return s.Parts[0].Text
}

// Int is an integer literal.
type Int struct {
	Base
	Value int64
}

// Float is a floating point literal.
type Float struct {
	Base
	Value float64
}

// Bool is a true or false literal.
type Bool struct {
	Base
	Value bool
}

// Null is the null literal.
type Null struct{ Base }

// Self is a relative reference: "." is the enclosing mapping, ".." its parent,
// and so on. Up counts how many mappings to walk outwards.
type Self struct {
	Base
	Up int
}

// Root is "$", the root node of the enclosing document.
type Root struct{ Base }

// Context is "$context" or its "$$" shorthand: the merged context data.
type Context struct{ Base }

// Field is an "x.name" access.
type Field struct {
	Base
	X    Node
	Name string
	// Optional records the "?." form, which yields null instead of failing
	// when x is null or has no such field.
	Optional bool
}

// Index is an "x[i]" access.
type Index struct {
	Base
	X     Node
	Index Node
	// Optional records the "?[" form, which yields null instead of failing
	// when x is null or has nothing at that index.
	Optional bool
}

// Coalesce is "x ?? y", which evaluates to y when x is null.
type Coalesce struct {
	Base
	X Node
	Y Node
}

// Ident is a bare identifier, which names a "local" binding.
type Ident struct {
	Base
	Name string
}
