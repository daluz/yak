// Package lexer turns yak source text into a flat slice of tokens.
//
// Indentation is deliberately not resolved here. YAML allows a block sequence
// to start at the same column as the mapping key that owns it, which no
// INDENT/DEDENT token scheme can express cleanly, so every token simply
// carries its line and column and the parser compares them.
package lexer

import (
	"fmt"
	"strings"

	"github.com/daluz/yak/internal/token"
)

// Error is a lexical error tied to a source position.
type Error struct {
	Pos token.Pos
	Msg string
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }

// Lex tokenizes a complete yak source file.
func Lex(file string, src []byte) ([]token.Token, error) {
	l := &lexer{
		file:  file,
		src:   src,
		line:  1,
		col:   1,
		bol:   true,
		block: true,
	}
	return l.run()
}

// LexExpr tokenizes an expression fragment, such as the body of a string
// interpolation. Positions are reported relative to start so that diagnostics
// point into the enclosing file.
func LexExpr(src string, start token.Pos) ([]token.Token, error) {
	l := &lexer{
		file:  start.File,
		src:   []byte(src),
		line:  start.Line,
		col:   start.Col,
		bol:   false,
		block: false,
	}
	return l.run()
}

type lexer struct {
	file string
	src  []byte
	off  int
	line int
	col  int

	// bol reports whether only whitespace has been seen on the current line.
	bol bool
	// block reports whether document markers and block scalars are allowed.
	block bool

	toks []token.Token
}

func (l *lexer) pos() token.Pos {
	return token.Pos{File: l.file, Line: l.line, Col: l.col}
}

func (l *lexer) errorf(p token.Pos, format string, args ...any) error {
	return &Error{Pos: p, Msg: fmt.Sprintf(format, args...)}
}

func (l *lexer) eof() bool { return l.off >= len(l.src) }

func (l *lexer) peek() byte {
	if l.off < len(l.src) {
		return l.src[l.off]
	}
	return 0
}

func (l *lexer) peekAt(n int) byte {
	if l.off+n < len(l.src) {
		return l.src[l.off+n]
	}
	return 0
}

func (l *lexer) advance() byte {
	c := l.src[l.off]
	l.off++
	if c == '\n' {
		l.line++
		l.col = 1
		l.bol = true
	} else {
		l.col++
	}
	return c
}

// emit records a token. It is always called immediately after the token's
// characters have been consumed, so the current position is its end.
func (l *lexer) emit(t token.Token) {
	t.End = l.pos()
	l.toks = append(l.toks, t)
	l.bol = false
}

func (l *lexer) run() ([]token.Token, error) {
	for {
		if err := l.skipSpace(); err != nil {
			return nil, err
		}
		if l.eof() {
			break
		}
		if err := l.scanToken(); err != nil {
			return nil, err
		}
	}
	l.toks = append(l.toks, token.Token{Kind: token.EOF, Pos: l.pos()})
	return l.toks, nil
}

func (l *lexer) skipSpace() error {
	for !l.eof() {
		c := l.peek()
		switch {
		case c == ' ' || c == '\r' || c == '\n':
			l.advance()
		case c == '\t':
			if l.bol {
				return l.errorf(l.pos(), "tabs may not be used for indentation")
			}
			l.advance()
		case c == '#' && (l.bol || l.prevIsSpace()):
			for !l.eof() && l.peek() != '\n' {
				l.advance()
			}
		default:
			return nil
		}
	}
	return nil
}

func (l *lexer) prevIsSpace() bool {
	if l.off == 0 {
		return true
	}
	c := l.src[l.off-1]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func (l *lexer) scanToken() error {
	start := l.pos()
	c := l.peek()

	if l.block && l.bol && l.col == 1 {
		if marker, ok := l.docMarker(); ok {
			kind := token.DocStart
			if marker == "..." {
				kind = token.DocEnd
			}
			l.advance()
			l.advance()
			l.advance()
			l.emit(token.Token{Kind: kind, Lit: marker, Pos: start})
			return nil
		}
	}

	switch {
	case c == '"' || c == '\'':
		return l.scanQuoted(false)
	case c == 'r' && isStringStart(l.peekAt(1)):
		l.advance()
		if l.peek() == '|' || l.peek() == '>' {
			return l.scanBlockScalar(start, true)
		}
		return l.scanQuotedAt(start, true)
	case c == '|' || c == '>':
		if !l.block {
			return l.errorf(start, "unexpected %q", string(c))
		}
		return l.scanBlockScalar(start, false)
	case isIdentStart(c):
		return l.scanIdent()
	case isDigit(c):
		return l.scanNumber()
	case c == '-':
		n := l.peekAt(1)
		if isDigit(n) {
			return l.scanNumber()
		}
		l.advance()
		l.emit(token.Token{Kind: token.Dash, Lit: "-", Pos: start})
		return nil
	case c == '.':
		n := 0
		for l.peek() == '.' {
			l.advance()
			n++
		}
		l.emit(token.Token{Kind: token.Dots, Lit: strings.Repeat(".", n), Pos: start})
		return nil
	case c == '$':
		l.advance()
		if l.peek() == '$' {
			l.advance()
			l.emit(token.Token{Kind: token.DoubleDollar, Lit: "$$", Pos: start})
			return nil
		}
		if isIdentStart(l.peek()) {
			name := l.readIdent()
			l.emit(token.Token{Kind: token.DollarIdent, Lit: name, Pos: start})
			return nil
		}
		l.emit(token.Token{Kind: token.Dollar, Lit: "$", Pos: start})
		return nil
	case c == ':':
		l.advance()
		if l.peek() == ':' {
			l.advance()
			l.emit(token.Token{Kind: token.DoubleColon, Lit: "::", Pos: start})
			return nil
		}
		l.emit(token.Token{Kind: token.Colon, Lit: ":", Pos: start})
		return nil
	case c == '=':
		l.advance()
		l.emit(token.Token{Kind: token.Assign, Lit: "=", Pos: start})
		return nil
	case c == ',':
		l.advance()
		l.emit(token.Token{Kind: token.Comma, Lit: ",", Pos: start})
		return nil
	case c == '[':
		l.advance()
		l.emit(token.Token{Kind: token.LBracket, Lit: "[", Pos: start})
		return nil
	case c == ']':
		l.advance()
		l.emit(token.Token{Kind: token.RBracket, Lit: "]", Pos: start})
		return nil
	case c == '{':
		l.advance()
		l.emit(token.Token{Kind: token.LBrace, Lit: "{", Pos: start})
		return nil
	case c == '}':
		l.advance()
		l.emit(token.Token{Kind: token.RBrace, Lit: "}", Pos: start})
		return nil
	case c == '(':
		l.advance()
		l.emit(token.Token{Kind: token.LParen, Lit: "(", Pos: start})
		return nil
	case c == ')':
		l.advance()
		l.emit(token.Token{Kind: token.RParen, Lit: ")", Pos: start})
		return nil
	case c == '?':
		switch l.peekAt(1) {
		case '?':
			l.advance()
			l.advance()
			l.emit(token.Token{Kind: token.Coalesce, Lit: "??", Pos: start})
			return nil
		case '.', '[':
			l.advance()
			l.emit(token.Token{Kind: token.Question, Lit: "?", Pos: start})
			return nil
		}
		return l.errorf(start,
			"explicit key indicators (%q) are not supported; use [expr] for a computed key, %q for an optional access, or %q for a default",
			"?", "?.", "??")
	case c == '&' || c == '*':
		return l.errorf(start, "anchors and aliases are not supported in yak; use a local variable instead")
	case c == '!':
		return l.errorf(start, "tags are not supported in yak yet; they will arrive with schemas")
	case c == '%':
		return l.errorf(start, "directives (%q) are not supported", "%")
	default:
		return l.errorf(start, "unexpected character %q", string(rune(c)))
	}
}

// docMarker reports a "---" or "..." marker at the start of a line.
func (l *lexer) docMarker() (string, bool) {
	if l.off+3 > len(l.src) {
		return "", false
	}
	s := string(l.src[l.off : l.off+3])
	if s != "---" && s != "..." {
		return "", false
	}
	if l.off+3 < len(l.src) {
		switch l.src[l.off+3] {
		case ' ', '\t', '\n', '\r':
		default:
			return "", false
		}
	}
	return s, true
}

func (l *lexer) readIdent() string {
	var sb strings.Builder
	sb.WriteByte(l.advance())
	for !l.eof() {
		c := l.peek()
		if isIdentPart(c) {
			sb.WriteByte(l.advance())
			continue
		}
		// A hyphen belongs to the identifier only when it joins two word
		// characters, keeping kebab-case keys usable without stealing the
		// minus sign from future arithmetic.
		if c == '-' && isIdentPart(l.peekAt(1)) {
			sb.WriteByte(l.advance())
			continue
		}
		break
	}
	return sb.String()
}

func (l *lexer) scanIdent() error {
	start := l.pos()
	name := l.readIdent()
	l.emit(token.Token{Kind: token.Ident, Lit: name, Pos: start})
	return nil
}

func (l *lexer) scanNumber() error {
	start := l.pos()
	var sb strings.Builder
	if l.peek() == '-' {
		sb.WriteByte(l.advance())
	}
	for isDigit(l.peek()) {
		sb.WriteByte(l.advance())
	}
	isFloat := false
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		isFloat = true
		sb.WriteByte(l.advance())
		for isDigit(l.peek()) {
			sb.WriteByte(l.advance())
		}
	}
	if c := l.peek(); c == 'e' || c == 'E' {
		next := l.peekAt(1)
		if isDigit(next) || ((next == '+' || next == '-') && isDigit(l.peekAt(2))) {
			isFloat = true
			sb.WriteByte(l.advance())
			if c := l.peek(); c == '+' || c == '-' {
				sb.WriteByte(l.advance())
			}
			for isDigit(l.peek()) {
				sb.WriteByte(l.advance())
			}
		}
	}
	if isIdentStart(l.peek()) {
		return l.errorf(l.pos(), "unexpected character %q after number; strings must be quoted", string(rune(l.peek())))
	}
	kind := token.Int
	if isFloat {
		kind = token.Float
	}
	l.emit(token.Token{Kind: kind, Lit: sb.String(), Pos: start})
	return nil
}

func isStringStart(c byte) bool {
	return c == '"' || c == '\'' || c == '|' || c == '>'
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
