// Package token defines the lexical tokens of the yak language and the source
// positions attached to them.
package token

import (
	"fmt"
	"strconv"
)

// Pos identifies a single character position in a source file. Lines and
// columns are 1-based.
type Pos struct {
	File string
	Line int
	Col  int
}

// IsValid reports whether the position refers to a real location.
func (p Pos) IsValid() bool { return p.Line > 0 }

func (p Pos) String() string {
	if !p.IsValid() {
		return "<unknown>"
	}
	if p.File == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Col)
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// Kind enumerates the kinds of token the lexer produces.
type Kind int

const (
	EOF Kind = iota
	DocStart
	DocEnd
	Ident
	DollarIdent
	Int
	Float
	String
	Colon
	DoubleColon
	// ColonQuestion is the ":?" of an entry hidden only when its value is
	// null.
	ColonQuestion
	// Dash is the "-" of a sequence item, of a subtraction, and of a
	// negation. Which one it is depends on where it appears.
	Dash
	Plus
	// Merge is the "<<" of a deep merge, written either between two
	// mappings or where the key of an entry belongs.
	Merge
	Star
	Slash
	Percent
	Comma
	LBracket
	RBracket
	LBrace
	RBrace
	LParen
	RParen
	Dots
	Dollar
	DoubleDollar
	// Assign separates the name and the value of a "local" binding.
	Assign
	// Arrow separates the parameters and the body of an anonymous function.
	Arrow
	// Coalesce is the "??" operator.
	Coalesce
	// Question is the "?" of an optional access, always followed directly by
	// "." or "[".
	Question
	// Not is the "!" of a negated boolean.
	Not
	And
	Or
	Eq
	Ne
	Lt
	Le
	Gt
	Ge
)

// spelling is the source text of every kind that is written exactly one way.
// Dots is listed by its shortest form: a run of them is written as many.
var spelling = map[Kind]string{
	DocStart:      "---",
	DocEnd:        "...",
	Colon:         ":",
	DoubleColon:   "::",
	ColonQuestion: ":?",
	Dash:          "-",
	Plus:          "+",
	Merge:         "<<",
	Star:          "*",
	Slash:         "/",
	Percent:       "%",
	Comma:         ",",
	LBracket:      "[",
	RBracket:      "]",
	LBrace:        "{",
	RBrace:        "}",
	LParen:        "(",
	RParen:        ")",
	Dots:          ".",
	Dollar:        "$",
	DoubleDollar:  "$$",
	Assign:        "=",
	Arrow:         "=>",
	Coalesce:      "??",
	Question:      "?",
	Not:           "!",
	And:           "&&",
	Or:            "||",
	Eq:            "==",
	Ne:            "!=",
	Lt:            "<",
	Le:            "<=",
	Gt:            ">",
	Ge:            ">=",
}

// described names the kinds whose text varies, which a diagnostic has to
// describe rather than quote.
var described = map[Kind]string{
	EOF:         "end of file",
	Ident:       "identifier",
	DollarIdent: "builtin variable",
	Int:         "integer",
	Float:       "float",
	String:      "string",
}

// Text is the source spelling of a fixed kind, and "" for the kinds whose
// text varies from one token to the next.
func (k Kind) Text() string { return spelling[k] }

func (k Kind) String() string {
	if s, ok := spelling[k]; ok {
		return strconv.Quote(s)
	}
	if d, ok := described[k]; ok {
		return d
	}
	return fmt.Sprintf("token(%d)", int(k))
}

// IsSeparator reports whether the kind separates a mapping key from its
// value.
func (k Kind) IsSeparator() bool {
	switch k {
	case Colon, DoubleColon, ColonQuestion:
		return true
	}
	return false
}

// Chunk is one piece of a string literal: either literal text or an
// interpolated expression written as { ... }.
type Chunk struct {
	// Text holds the decoded literal text when IsExpr is false, and the raw
	// expression source when IsExpr is true.
	Text   string
	IsExpr bool
	// Pos points at the first character of Text in the source file, so that
	// diagnostics inside interpolations report useful locations.
	Pos Pos
}

// Comment is a "#" comment. Comments are kept out of the token stream so
// that the parser never has to skip over one, and are matched back to the
// nodes they belong to afterwards.
type Comment struct {
	// Text is the comment as written, including the leading "#" and
	// without any trailing whitespace.
	Text string
	Pos  Pos
	// OwnLine reports that nothing but whitespace preceded the comment on
	// its line, which makes it describe what follows rather than what it
	// was written after.
	OwnLine bool
	// Next is the index of the first token after the comment, which is how
	// a comment is placed relative to the constructs around it.
	Next int
}

// Token is a single lexical unit.
type Token struct {
	Kind Kind
	// Lit is the decoded literal: the identifier name, the numeric text, or
	// the fully decoded string when the string has no interpolations.
	Lit string
	// Chunks is set for String tokens that contain interpolations.
	Chunks []Chunk
	Pos    Pos
	// End is the position just past the token's last character. It lets the
	// parser require that references such as ".name" are written without
	// intervening whitespace.
	End Pos
}

// Line reports the line the token starts on.
func (t Token) Line() int { return t.Pos.Line }

// Col reports the column the token starts on.
func (t Token) Col() int { return t.Pos.Col }

func (t Token) String() string {
	switch t.Kind {
	case Ident, Int, Float, Dots:
		return strconv.Quote(t.Lit)
	case String:
		return "string"
	case DollarIdent:
		return strconv.Quote("$" + t.Lit)
	default:
		return t.Kind.String()
	}
}

// Keywords reserved by the language. They lex as identifiers; the parser
// decides what they mean. "local", "import" and "schema" each begin a
// statement rather than a value.
const (
	KeywordTrue   = "true"
	KeywordFalse  = "false"
	KeywordNull   = "null"
	KeywordLocal  = "local"
	KeywordImport = "import"
	KeywordSchema = "schema"
)

// Keywords that mean something only where a value is expected. They are not
// reserved, so "for" and "else" remain usable as mapping keys.
const (
	KeywordIf   = "if"
	KeywordThen = "then"
	KeywordElse = "else"
	KeywordFor  = "for"
	KeywordIn   = "in"
)

// IsReserved reports whether name is a reserved word that may not be used as a
// plain value identifier.
func IsReserved(name string) bool {
	switch name {
	case KeywordTrue, KeywordFalse, KeywordNull, KeywordLocal, KeywordImport, KeywordSchema:
		return true
	}
	return false
}

// IsKeyword reports whether name is one of the contextual keywords. They may
// be written as keys but cannot name a binding or a loop variable, because
// reading such a name back would be read as the keyword instead.
func IsKeyword(name string) bool {
	switch name {
	case KeywordIf, KeywordThen, KeywordElse, KeywordFor, KeywordIn:
		return true
	}
	return false
}
