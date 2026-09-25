package parser

import (
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/lexer"
	"github.com/daluz/yak/internal/token"
)

// parseInlineValue parses a value that appears on a single line: a literal, a
// reference, or a flow collection.
func (p *parser) parseInlineValue() (ast.Node, error) { return p.parseExpr() }

func (p *parser) parseExpr() (ast.Node, error) {
	x, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	return p.parsePostfix(x)
}

func (p *parser) parsePrimary() (ast.Node, error) {
	t := p.cur()
	switch t.Kind {
	case token.Int:
		p.next()
		v, err := strconv.ParseInt(t.Lit, 10, 64)
		if err != nil {
			return nil, p.errorf(t.Pos, "integer %q is out of range", t.Lit)
		}
		return &ast.Int{Base: ast.At(t.Pos), Value: v}, nil

	case token.Float:
		p.next()
		v, err := strconv.ParseFloat(t.Lit, 64)
		if err != nil {
			return nil, p.errorf(t.Pos, "invalid float %q", t.Lit)
		}
		return &ast.Float{Base: ast.At(t.Pos), Value: v}, nil

	case token.String:
		p.next()
		return p.buildString(t)

	case token.Ident:
		return p.parseIdent()

	case token.Dots:
		return p.parseSelf()

	case token.Dollar:
		p.next()
		return &ast.Root{Base: ast.At(t.Pos)}, nil

	case token.DoubleDollar:
		p.next()
		return &ast.Context{Base: ast.At(t.Pos)}, nil

	case token.DollarIdent:
		p.next()
		if t.Lit != "context" {
			return nil, p.errorf(t.Pos, "unknown special variable %q; the only one is %q", "$"+t.Lit, "$context")
		}
		return &ast.Context{Base: ast.At(t.Pos)}, nil

	case token.LBracket:
		return p.parseFlowSequence()

	case token.LBrace:
		return p.parseFlowMapping()

	case token.LParen:
		p.next()
		x, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.at(token.RParen) {
			return nil, p.errorf(p.cur().Pos, "expected %q, found %s", ")", p.cur())
		}
		p.next()
		return x, nil

	default:
		return nil, p.errorf(t.Pos, "expected a value, found %s", t)
	}
}

// unsupportedKeywords maps reserved words that are planned but not yet
// implemented to the message shown when they are used.
var unsupportedKeywords = map[string]string{
	token.KeywordLocal:  `"local" bindings are not implemented yet`,
	token.KeywordImport: `"import" is not implemented yet`,
	token.KeywordSchema: `"schema" is not implemented yet`,
}

// yamlBooleans are the extra boolean spellings YAML accepts and yak does not.
var yamlBooleans = map[string]string{
	"yes": "true", "no": "false",
	"on": "true", "off": "false",
	"y": "true", "n": "false",
}

func (p *parser) parseIdent() (ast.Node, error) {
	t := p.next()
	switch t.Lit {
	case token.KeywordTrue:
		return &ast.Bool{Base: ast.At(t.Pos), Value: true}, nil
	case token.KeywordFalse:
		return &ast.Bool{Base: ast.At(t.Pos), Value: false}, nil
	case token.KeywordNull:
		return &ast.Null{Base: ast.At(t.Pos)}, nil
	}
	if msg, ok := unsupportedKeywords[t.Lit]; ok {
		return nil, p.errorf(t.Pos, "%s", msg)
	}
	if want, ok := yamlBooleans[strings.ToLower(t.Lit)]; ok {
		return nil, p.errorf(t.Pos, "%q is not a boolean in yak; write %s, or quote it to make it a string", t.Lit, want)
	}
	return &ast.Ident{Base: ast.At(t.Pos), Name: t.Lit}, nil
}

// parseSelf parses a leading run of dots. A run of n dots walks n-1 mappings
// outwards, so "." is the current mapping and ".." is its parent. A field name
// written immediately after the dots binds to that mapping.
func (p *parser) parseSelf() (ast.Node, error) {
	dots := p.next()
	self := &ast.Self{Base: ast.At(dots.Pos), Up: len(dots.Lit) - 1}
	if name, ok := p.adjacentFieldName(dots); ok {
		return &ast.Field{Base: ast.At(dots.Pos), X: self, Name: name}, nil
	}
	return self, nil
}

// adjacentFieldName consumes an identifier that follows dots with no
// intervening whitespace.
func (p *parser) adjacentFieldName(dots token.Token) (string, bool) {
	t := p.cur()
	if t.Kind != token.Ident || t.Pos != dots.End {
		return "", false
	}
	p.next()
	return t.Lit, true
}

// parsePostfix applies field and index accesses. Each must be written flush
// against the expression it applies to, so "$.b .c" is a mistake rather than
// a silently accepted "$.b.c".
func (p *parser) parsePostfix(x ast.Node) (ast.Node, error) {
	for {
		t := p.cur()
		if !p.adjacent() {
			return x, nil
		}
		switch t.Kind {
		case token.Dots:
			if len(t.Lit) > 1 {
				return nil, p.errorf(t.Pos, "%q may only begin a reference, not continue one", t.Lit)
			}
			dots := p.next()
			name, ok := p.adjacentFieldName(dots)
			if !ok {
				return nil, p.errorf(p.cur().Pos, "expected a field name after %q", ".")
			}
			x = &ast.Field{Base: ast.At(x.Pos()), X: x, Name: name}
		case token.LBracket:
			p.next()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.at(token.RBracket) {
				return nil, p.errorf(p.cur().Pos, "expected %q to close an index, found %s", "]", p.cur())
			}
			p.next()
			x = &ast.Index{Base: ast.At(x.Pos()), X: x, Index: idx}
		default:
			return x, nil
		}
	}
}

func (p *parser) parseFlowSequence() (ast.Node, error) {
	open := p.next()
	s := &ast.Sequence{Base: ast.At(open.Pos), Flow: true}
	for {
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated flow sequence: missing %q", "]")
		}
		if p.at(token.RBracket) {
			p.next()
			return s, nil
		}
		item, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		s.Items = append(s.Items, item)
		if p.at(token.Comma) {
			p.next()
			continue
		}
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated flow sequence: missing %q", "]")
		}
		if !p.at(token.RBracket) {
			return nil, p.errorf(p.cur().Pos, "expected %q or %q in flow sequence, found %s", ",", "]", p.cur())
		}
	}
}

func (p *parser) parseFlowMapping() (ast.Node, error) {
	open := p.next()
	m := &ast.Mapping{Base: ast.At(open.Pos), Flow: true}
	for {
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated flow mapping: missing %q", "}")
		}
		if p.at(token.RBrace) {
			p.next()
			return m, nil
		}
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
		p.next()
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		m.Entries = append(m.Entries, &ast.Entry{
			Key: key, Computed: computed, Hidden: hidden, Value: value, KeyPos: keyPos,
		})
		if p.at(token.Comma) {
			p.next()
			continue
		}
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated flow mapping: missing %q", "}")
		}
		if !p.at(token.RBrace) {
			return nil, p.errorf(p.cur().Pos, "expected %q or %q in flow mapping, found %s", ",", "}", p.cur())
		}
	}
}

// buildString converts a string token into an ast.String, parsing each
// interpolation as an independent expression.
func (p *parser) buildString(t token.Token) (*ast.String, error) {
	s := &ast.String{Base: ast.At(t.Pos)}
	if t.Chunks == nil {
		s.Parts = []ast.StringPart{{Text: t.Lit}}
		return s, nil
	}
	for _, c := range t.Chunks {
		if !c.IsExpr {
			s.Parts = append(s.Parts, ast.StringPart{Text: c.Text})
			continue
		}
		expr, err := p.parseFragment(c.Text, c.Pos)
		if err != nil {
			return nil, err
		}
		s.Parts = append(s.Parts, ast.StringPart{Expr: expr})
	}
	return s, nil
}

// parseFragment parses the body of a string interpolation.
func (p *parser) parseFragment(src string, start token.Pos) (ast.Node, error) {
	toks, err := lexer.LexExpr(src, start)
	if err != nil {
		return nil, err
	}
	sub := &parser{file: p.file, toks: toks}
	if sub.at(token.EOF) {
		return nil, p.errorf(start, "empty string interpolation")
	}
	expr, err := sub.parseExpr()
	if err != nil {
		return nil, err
	}
	if !sub.at(token.EOF) {
		return nil, p.errorf(sub.cur().Pos, "unexpected %s in string interpolation", sub.cur())
	}
	return expr, nil
}
