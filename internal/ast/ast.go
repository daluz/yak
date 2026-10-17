// Package ast defines the syntax tree produced by the yak parser.
package ast

import (
	"strings"

	"github.com/daluz/yak/internal/token"
)

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
	// Binds holds the "let" and "import" bindings written among the entries. They cover
	// the whole mapping, wherever in it they appear, and they can see the
	// mapping itself, so a binding may refer to a field with ".name".
	Binds   []*Binding
	Entries []*Entry
	Flow    bool
}

// Binding is one "name = value" pair introduced by "let", or one module
// introduced by "import", whose Value is then an *Import. A binding written
// as "[a, b] = value" has a Pattern instead of a Name.
type Binding struct {
	Base
	// Comments are the ones written on the binding itself. A binding
	// renders nothing of its own, so they come out wherever it is read
	// back instead.
	Comments
	Name    string
	Pattern *Pattern
	Value   Node
}

// Wildcard is the name a pattern writes for an item it binds nothing to.
const Wildcard = "_"

// Pattern is what a "for" or a "let" binds a value to: a name, the
// wildcard, or a bracketed list of patterns, which destructures a sequence
// of exactly that many items.
type Pattern struct {
	Base
	// Name is the name bound, or Wildcard. It is empty for a list.
	Name string
	// Items are the patterns of a list, and nil for anything else.
	Items []*Pattern
}

// String writes the pattern as it would be written in a template.
func (p *Pattern) String() string {
	if p.Items == nil {
		return p.Name
	}
	parts := make([]string, len(p.Items))
	for i, item := range p.Items {
		parts[i] = item.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Walk calls f with each name the pattern binds, in the order they are
// written, along with the indices of the items that lead to it.
func (p *Pattern) Walk(f func(name *Pattern, path []int)) {
	p.walk(nil, f)
}

func (p *Pattern) walk(path []int, f func(*Pattern, []int)) {
	switch {
	case p.Items != nil:
		for i, item := range p.Items {
			item.walk(append(path[:len(path):len(path)], i), f)
		}
	case p.Name != Wildcard:
		f(p, path)
	}
}

// Function is the value of a binding whose name was followed by a parameter
// list, or of a "(params) => body" written where a value belongs. Name is the
// name of the binding, which is what diagnostics about a call report, and is
// empty for the anonymous form.
type Function struct {
	Base
	Name   string
	Params []*Param
	Body   Node
}

// Param is one parameter of a Function. Default is nil for a parameter that
// every call has to supply. A parameter written as "[a, b]" has a Pattern
// instead of a Name, and can only be given by position.
type Param struct {
	Base
	Name    string
	Pattern *Pattern
	Default Node
}

// Label is how a diagnostic names the parameter: its name, or the pattern
// it was written as.
func (p *Param) Label() string {
	if p.Pattern != nil {
		return p.Pattern.String()
	}
	return p.Name
}

// Import is the value of a binding introduced by "import": the module that the
// dotted path names, read from the file the import is written in.
type Import struct {
	Base
	Path []string
}

// Let scopes bindings over a body that is not a mapping. Bindings written
// in a mapping belong to the Mapping instead, so that they can refer to its
// fields.
type Let struct {
	Base
	Binds []*Binding
	Body  Node
}

// Comments are the source comments attached to a node. The renderer writes
// them back out around the entry or item that ends up carrying them.
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

// Entry is one key/value pair of a Mapping, or the "<<" of a merge written
// where a key belongs.
type Entry struct {
	Comments
	// Key is a *String for literal keys (bare identifiers are normalized into
	// strings) or an arbitrary expression for computed keys written as (expr).
	// A merge entry has no key.
	Key Node
	// Computed records that the key was written in parentheses.
	Computed bool
	// Merge records that the entry was written "<<" instead of with a key,
	// which merges the mapping it names into the one being built rather
	// than adding an entry of its own.
	Merge bool
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

// Unary is "not x" or "-x".
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

// Loop is the "for pattern in source" clause of a comprehension, together
// with the optional "if" filter that decides which items it keeps. The
// unbracketed "for a, b in source" is held as the list pattern [a, b].
type Loop struct {
	Base
	Pattern *Pattern
	Source  Node
	Filter  Node
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

// Ident is a bare identifier, which names a "let" binding.
type Ident struct {
	Base
	Name string
}
