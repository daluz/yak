// Package parser builds an abstract syntax tree from yak source.
//
// Block structure is resolved by comparing token columns rather than by
// consuming INDENT/DEDENT tokens, because YAML permits a block sequence to
// begin at the same column as the mapping key that owns it.
package parser

import (
	"fmt"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/lexer"
	"github.com/daluz/yak/internal/token"
)

// Error is a syntax error tied to a source position.
type Error struct {
	Pos token.Pos
	Msg string
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }

// Parse parses a complete yak source file into a document stream.
func Parse(file string, src []byte) (*ast.Stream, error) {
	toks, err := lexer.Lex(file, src)
	if err != nil {
		return nil, err
	}
	p := &parser{file: file, toks: toks}
	return p.parseStream()
}

type parser struct {
	file string
	toks []token.Token
	i    int
	// prevEnd is where the most recently consumed token ended, used to
	// require that references are written without internal whitespace.
	prevEnd token.Pos
}

func (p *parser) cur() token.Token { return p.toks[p.i] }

func (p *parser) at(k token.Kind) bool { return p.cur().Kind == k }

func (p *parser) next() token.Token {
	t := p.toks[p.i]
	p.prevEnd = t.End
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

// adjacent reports whether the current token starts exactly where the
// previous one ended.
func (p *parser) adjacent() bool {
	return p.cur().Pos == p.prevEnd
}

func (p *parser) errorf(pos token.Pos, format string, args ...any) error {
	return &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// atDocBoundary reports whether the current token ends the current document.
func (p *parser) atDocBoundary() bool {
	switch p.cur().Kind {
	case token.EOF, token.DocStart, token.DocEnd:
		return true
	}
	return false
}

func (p *parser) parseStream() (*ast.Stream, error) {
	st := &ast.Stream{File: p.file}
	for !p.at(token.EOF) {
		if p.at(token.DocEnd) {
			p.next()
			continue
		}
		explicit := false
		if p.at(token.DocStart) {
			p.next()
			explicit = true
		}
		pos := p.cur().Pos
		if p.atDocBoundary() {
			if explicit {
				st.Docs = append(st.Docs, &ast.Document{Base: ast.At(pos), Body: &ast.Null{Base: ast.At(pos)}})
			}
			continue
		}
		body, err := p.parseBlockNode()
		if err != nil {
			return nil, err
		}
		st.Docs = append(st.Docs, &ast.Document{Base: ast.At(pos), Body: body})
		if !p.atDocBoundary() {
			return nil, p.errorf(p.cur().Pos, "unexpected %s at document level", p.cur())
		}
	}
	if len(st.Docs) == 0 {
		pos := token.Pos{File: p.file, Line: 1, Col: 1}
		st.Docs = append(st.Docs, &ast.Document{Base: ast.At(pos), Body: &ast.Null{Base: ast.At(pos)}})
	}
	return st, nil
}

// parseBlockNode parses a node in block context. The node's own extent is
// fixed by the column of its first token: a mapping ends at the first key that
// is less indented, and a sequence at the first dash that is.
func (p *parser) parseBlockNode() (ast.Node, error) {
	t := p.cur()
	if t.Kind == token.Dash {
		return p.parseBlockSequence(t.Col())
	}
	if p.looksLikeEntry() {
		return p.parseBlockMapping(t.Col())
	}
	return p.parseInlineValue()
}

func (p *parser) parseBlockMapping(col int) (ast.Node, error) {
	m := &ast.Mapping{Base: ast.At(p.cur().Pos)}
	for {
		if p.atDocBoundary() {
			break
		}
		t := p.cur()
		if t.Col() < col {
			break
		}
		if t.Col() > col {
			return nil, p.errorf(t.Pos, "unexpected indentation: expected a mapping key at column %d", col)
		}
		if !p.looksLikeEntry() {
			return nil, p.errorf(t.Pos, "expected a mapping key, found %s", t)
		}
		entry, err := p.parseMappingEntry(col)
		if err != nil {
			return nil, err
		}
		m.Entries = append(m.Entries, entry)
	}
	return m, nil
}

func (p *parser) parseMappingEntry(col int) (*ast.Entry, error) {
	keyPos := p.cur().Pos
	key, computed, err := p.parseKey()
	if err != nil {
		return nil, err
	}
	var hidden bool
	switch p.cur().Kind {
	case token.Colon:
	case token.DoubleColon:
		hidden = true
	default:
		return nil, p.errorf(p.cur().Pos, "expected %q or %q after mapping key, found %s", ":", "::", p.cur())
	}
	colon := p.next()
	value, err := p.parseEntryValue(col, colon)
	if err != nil {
		return nil, err
	}
	return &ast.Entry{Key: key, Computed: computed, Hidden: hidden, Value: value, KeyPos: keyPos}, nil
}

// parseEntryValue parses the value that follows a mapping key, which is either
// on the same line as the colon or in a more indented block below it.
func (p *parser) parseEntryValue(parentCol int, colon token.Token) (ast.Node, error) {
	if p.atDocBoundary() {
		return &ast.Null{Base: ast.At(colon.Pos)}, nil
	}
	t := p.cur()
	if t.Line() == colon.Line() {
		v, err := p.parseInlineValue()
		if err != nil {
			return nil, err
		}
		if !p.atDocBoundary() && p.cur().Line() == colon.Line() {
			return nil, p.errorf(p.cur().Pos, "unexpected %s after value; strings must be quoted", p.cur())
		}
		return v, nil
	}
	if t.Kind == token.Dash && t.Col() >= parentCol {
		return p.parseBlockSequence(t.Col())
	}
	if t.Col() > parentCol {
		return p.parseBlockNode()
	}
	return &ast.Null{Base: ast.At(colon.Pos)}, nil
}

func (p *parser) parseBlockSequence(col int) (ast.Node, error) {
	s := &ast.Sequence{Base: ast.At(p.cur().Pos)}
	for p.cur().Kind == token.Dash && p.cur().Col() == col {
		dash := p.next()
		item, err := p.parseSequenceItem(dash)
		if err != nil {
			return nil, err
		}
		s.Items = append(s.Items, item)
	}
	return s, nil
}

func (p *parser) parseSequenceItem(dash token.Token) (ast.Node, error) {
	if p.atDocBoundary() {
		return &ast.Null{Base: ast.At(dash.Pos)}, nil
	}
	t := p.cur()
	if t.Line() == dash.Line() {
		if t.Col() <= dash.Col() {
			return nil, p.errorf(t.Pos, "unexpected %s after %q", t, "-")
		}
		return p.parseBlockNode()
	}
	if t.Col() > dash.Col() {
		return p.parseBlockNode()
	}
	return &ast.Null{Base: ast.At(dash.Pos)}, nil
}

// looksLikeEntry reports whether the current position begins a mapping entry.
// It scans for a key-shaped token run followed by ":" or "::" without
// validating the key, so that an invalid key still produces the specific
// diagnostic from parseKey rather than being silently reparsed as a value.
func (p *parser) looksLikeEntry() bool {
	j := p.keyEnd(p.i)
	if j < 0 || j >= len(p.toks) {
		return false
	}
	k := p.toks[j].Kind
	return k == token.Colon || k == token.DoubleColon
}

// keyEnd returns the index just past a key-shaped run of tokens starting at i,
// or -1 if no key can start there.
func (p *parser) keyEnd(i int) int {
	switch p.toks[i].Kind {
	case token.Ident, token.String, token.Int, token.Float:
		return i + 1
	case token.LBracket:
		depth := 0
		for j := i; j < len(p.toks); j++ {
			switch p.toks[j].Kind {
			case token.LBracket:
				depth++
			case token.RBracket:
				depth--
				if depth == 0 {
					return j + 1
				}
			case token.EOF:
				return -1
			}
		}
	}
	return -1
}

// parseKey parses a mapping key: a bare identifier, a quoted string, or a
// computed key in brackets.
func (p *parser) parseKey() (ast.Node, bool, error) {
	t := p.cur()
	switch t.Kind {
	case token.Ident:
		if token.IsReserved(t.Lit) {
			return nil, false, p.errorf(t.Pos, "%q is a reserved word and must be quoted to be used as a key", t.Lit)
		}
		p.next()
		return &ast.String{Base: ast.At(t.Pos), Parts: []ast.StringPart{{Text: t.Lit}}}, false, nil
	case token.String:
		p.next()
		s, err := p.buildString(t)
		if err != nil {
			return nil, false, err
		}
		return s, false, nil
	case token.LBracket:
		p.next()
		key, err := p.parseExpr()
		if err != nil {
			return nil, false, err
		}
		if !p.at(token.RBracket) {
			return nil, false, p.errorf(p.cur().Pos, "expected %q to close a computed key, found %s", "]", p.cur())
		}
		p.next()
		return key, true, nil
	case token.Int, token.Float:
		return nil, false, p.errorf(t.Pos, "numeric keys must be quoted, for example %q", `"`+t.Lit+`"`)
	default:
		return nil, false, p.errorf(t.Pos, "expected a mapping key, found %s", t)
	}
}
