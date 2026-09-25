// Package token defines the lexical tokens of the yak language and the source
// positions attached to them.
package token

import "fmt"

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
	Dash
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
	// Assign is reserved for "local name = value" bindings. Accepting it in
	// the lexer lets the parser explain that they are not implemented yet
	// instead of failing on an unexpected character.
	Assign
)

var kindNames = map[Kind]string{
	EOF:          "end of file",
	DocStart:     `"---"`,
	DocEnd:       `"..."`,
	Ident:        "identifier",
	DollarIdent:  "special variable",
	Int:          "integer",
	Float:        "float",
	String:       "string",
	Colon:        `":"`,
	DoubleColon:  `"::"`,
	Dash:         `"-"`,
	Comma:        `","`,
	LBracket:     `"["`,
	RBracket:     `"]"`,
	LBrace:       `"{"`,
	RBrace:       `"}"`,
	LParen:       `"("`,
	RParen:       `")"`,
	Dots:         `"."`,
	Dollar:       `"$"`,
	DoubleDollar: `"$$"`,
	Assign:       `"="`,
}

func (k Kind) String() string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return fmt.Sprintf("token(%d)", int(k))
}

// Chunk is one piece of a string literal: either literal text or an
// interpolated expression written as ${ ... }.
type Chunk struct {
	// Text holds the decoded literal text when IsExpr is false, and the raw
	// expression source when IsExpr is true.
	Text   string
	IsExpr bool
	// Pos points at the first character of Text in the source file, so that
	// diagnostics inside interpolations report useful locations.
	Pos Pos
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
	case Ident, Int, Float:
		return fmt.Sprintf("%q", t.Lit)
	case String:
		return "string"
	case DollarIdent:
		return fmt.Sprintf("%q", "$"+t.Lit)
	case Dots:
		return fmt.Sprintf("%q", t.Lit)
	default:
		return t.Kind.String()
	}
}

// Keywords reserved by the language. They lex as identifiers; the parser
// decides what they mean.
const (
	KeywordTrue   = "true"
	KeywordFalse  = "false"
	KeywordNull   = "null"
	KeywordLocal  = "local"
	KeywordImport = "import"
	KeywordSchema = "schema"
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
