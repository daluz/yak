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

// Function is the value of a binding whose name was followed by a parameter
// list. Name is the name of that binding, which is what diagnostics about a
// call report.
type Function struct {
	Base
	Name   string
	Params []*Param
	Body   Node
}

// Param is one parameter of a Function. Default is nil for a parameter that
// every call has to supply.
type Param struct {
	Base
	Name    string
	Default Node
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
	// HideNull records that the entry was written with ":?", which omits it
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
	// Src is how the reference was written, either a run of dots or
	// "$self". A reference that reaches past the outermost mapping is
	// reported with the spelling it was written in.
	Src string
}

// Root is "$", or "$root" written out in full: the root node of the enclosing
// document.
type Root struct{ Base }

// Context is "$context" or its "$$" shorthand: the merged context data.
type Context struct{ Base }

// Yak is "$yak", a mapping that describes the run rather than the document:
// the version rendering it, the template's path, and the context files it
// was given.
type Yak struct{ Base }

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

// Call is an "f(x)" call of a Function.
type Call struct {
	Base
	Fn   Node
	Args []Arg
}

// Arg is one argument of a Call. Name is empty for a positional argument,
// which takes the parameter in the position it was written in.
type Arg struct {
	Name    string
	NamePos token.Pos
	Value   Node
}

// Coalesce is "x ?? y", which evaluates to y when x is null.
type Coalesce struct {
	Base
	X Node
	Y Node
}

// Unary is "!x".
type Unary struct {
	Base
	Op token.Kind
	X  Node
}

// Binary is a comparison or a logical operator applied to two operands.
type Binary struct {
	Base
	Op token.Kind
	// OpPos points at the operator, which is where a diagnostic about
	// mismatched operands belongs.
	OpPos token.Pos
	X     Node
	Y     Node
}

// If is "if cond then x else y". Else is nil when the branch was left out,
// which makes the conditional yield null instead.
type If struct {
	Base
	Cond Node
	Then Node
	Else Node
}

// Loop is the "for name in source" clause of a comprehension, together with
// the optional "if" filter that decides which items it keeps.
type Loop struct {
	Base
	Var    string
	VarPos token.Pos
	Source Node
	Filter Node
}

// SeqComp is a sequence comprehension, "[item for name in source]".
type SeqComp struct {
	Loop
	Item Node
}

// MapComp is a mapping comprehension, "{key: value for name in source}".
type MapComp struct {
	Loop
	Entry *Entry
}

// Ident is a bare identifier, which names a "local" binding.
type Ident struct {
	Base
	Name string
}
