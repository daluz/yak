package parser

import (
	"slices"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// parseSchema parses what follows "schema": a "*" written against the
// keyword, which makes what it declares open, and then one declaration or a
// braced block of them. start is the index of the keyword and head the
// comments above it.
//
// A schema renders nothing and is never read back as a value, so every
// comment written inside it is dropped rather than left for whatever comes
// next.
func (p *parser) parseSchema(start int, head []string, col int) ([]*ast.Binding, error) {
	open := false
	if p.at(token.Star) {
		if !p.adjacent() {
			return nil, p.errorf(p.cur().Pos, "an open schema is written %q, with the %q against the keyword", "schema*", "*")
		}
		p.next()
		open = true
	}
	if p.at(token.Star) {
		return nil, p.errorf(p.cur().Pos, "an open schema is written %q, with one %q", "schema*", "*")
	}
	var binds []*ast.Binding
	var err error
	if p.at(token.LBrace) {
		binds, err = p.parseBlock(token.KeywordSchema, head, func() (*ast.Binding, error) {
			return p.parseSchemaSpec(open, -1)
		})
	} else {
		var b *ast.Binding
		b, err = p.parseSchemaSpec(open, col)
		binds = []*ast.Binding{b}
	}
	p.comments.drop(start, p.i)
	return binds, err
}

// parseSchemaSpec parses one "Name = type", "$ = type" or "$$ = type". col
// is the column of the statement, whose type may then be a block below it,
// and is negative inside a "schema { ... }" block, where every type is
// written inline.
func (p *parser) parseSchemaSpec(open bool, col int) (*ast.Binding, error) {
	t := p.cur()
	var name string
	switch t.Kind {
	case token.Ident:
		if err := p.checkSchemaName(t); err != nil {
			return nil, err
		}
		name = t.Lit
	case token.Dollar, token.DoubleDollar:
		if p.nested > 0 {
			return nil, p.errorf(t.Pos, "%q can only be declared at the top of a document", "schema "+t.Kind.Text())
		}
		name = t.Kind.Text()
	default:
		return nil, p.errorf(t.Pos, "expected the name of a schema, %q or %q, found %s", "$", "$$", t)
	}
	p.next()
	if !p.at(token.Assign) {
		return nil, p.errorf(p.cur().Pos, "expected %q after the name of a schema, found %s", "=", p.cur())
	}
	assign := p.next()
	var typ ast.Node
	var err error
	if col < 0 {
		typ, err = p.parseType()
	} else {
		typ, err = p.parseSchemaBody(col, assign)
	}
	if err != nil {
		return nil, err
	}
	return &ast.Binding{Base: ast.At(t.Pos), Name: name, Value: &ast.Schema{Base: ast.At(t.Pos), Open: open, Type: typ}}, nil
}

// checkSchemaName rejects a name that cannot name a schema: a reserved word,
// a keyword, or a type every schema can already name.
func (p *parser) checkSchemaName(t token.Token) error {
	switch {
	case token.IsReserved(t.Lit):
		return p.errorf(t.Pos, "%q is a reserved word and cannot name a schema", t.Lit)
	case token.IsKeyword(t.Lit):
		return p.errorf(t.Pos, "%q is a keyword and cannot name a schema", t.Lit)
	case t.Lit == ast.Wildcard:
		return p.errorf(t.Pos, "%q discards a value and cannot name a schema", t.Lit)
	case slices.Contains(ast.BuiltinTypes, t.Lit):
		return p.errorf(t.Pos, "%q is a built-in type and cannot name a schema", t.Lit)
	}
	return nil
}

// parseSchemaBody parses the type after the "=" of a schema statement: one
// written on the same line, or a block mapping type indented below it.
func (p *parser) parseSchemaBody(col int, assign token.Token) (ast.Node, error) {
	if !p.atDocBoundary() && p.cur().Line() == assign.Line() {
		typ, err := p.parseType()
		if err != nil {
			return nil, err
		}
		return typ, p.endOfLine(assign.Line(), "type")
	}
	if p.atDocBoundary() || p.cur().Col() <= col {
		return nil, p.errorf(assign.Pos, "expected a type after %q", "=")
	}
	return p.parseTypeBlock(p.cur().Col())
}

// endOfLine rejects anything left on line after a what.
func (p *parser) endOfLine(line int, what string) error {
	if !p.atDocBoundary() && p.cur().Line() == line {
		return p.errorf(p.cur().Pos, "unexpected %s after a %s", p.cur(), what)
	}
	return nil
}

// parseTypeBlock parses a block mapping type whose members start at col.
func (p *parser) parseTypeBlock(col int) (*ast.TypeMapping, error) {
	m := &ast.TypeMapping{Base: ast.At(p.cur().Pos)}
	for !p.atDocBoundary() && p.cur().Col() >= col {
		if p.cur().Col() > col {
			return nil, p.errorf(p.cur().Pos, "unexpected indentation: expected a field of the schema at column %d", col)
		}
		member, sep, err := p.parseTypeMemberHead()
		if err != nil {
			return nil, err
		}
		if !p.atDocBoundary() && p.cur().Line() == sep.Line() {
			if err := p.parseTypeMemberValue(member); err != nil {
				return nil, err
			}
			if err := p.endOfLine(sep.Line(), "field of a schema"); err != nil {
				return nil, err
			}
		} else {
			if p.atDocBoundary() || p.cur().Col() <= col {
				return nil, p.errorf(sep.Pos, "expected a type after %s", sep)
			}
			if member.Type, err = p.parseTypeBlock(p.cur().Col()); err != nil {
				return nil, err
			}
		}
		m.Members = append(m.Members, member)
	}
	return m, nil
}

// parseFlowTypeMapping parses a mapping type written in braces.
func (p *parser) parseFlowTypeMapping() (*ast.TypeMapping, error) {
	open := p.next()
	m := &ast.TypeMapping{Base: ast.At(open.Pos)}
	for {
		more, err := p.nextInList(open, flowMapList, len(m.Members) == 0)
		if !more {
			return m, err
		}
		member, _, err := p.parseTypeMemberHead()
		if err != nil {
			return nil, err
		}
		if err := p.parseTypeMemberValue(member); err != nil {
			return nil, err
		}
		m.Members = append(m.Members, member)
	}
}

// parseTypeMemberHead parses everything a member of a mapping type has
// before its type: a field name, a key type in parentheses or a "<<", and
// the separator after it, which it answers.
func (p *parser) parseTypeMemberHead() (*ast.TypeMember, token.Token, error) {
	t := p.cur()
	member := &ast.TypeMember{Base: ast.At(t.Pos)}
	switch t.Kind {
	case token.Merge:
		p.next()
		member.Merge = true
	case token.LParen:
		p.next()
		key, err := p.parseType()
		if err != nil {
			return nil, t, err
		}
		if !p.at(token.RParen) {
			return nil, t, p.errorf(p.cur().Pos, "expected %q to close the type of a key, found %s", ")", p.cur())
		}
		p.next()
		member.Key = key
	case token.Ident:
		if token.IsReserved(t.Lit) {
			return nil, t, p.errorf(t.Pos, "%q is a reserved word and must be quoted to name a field", t.Lit)
		}
		p.next()
		member.Name = t.Lit
	case token.String:
		p.next()
		s, err := p.buildString(t)
		if err != nil {
			return nil, t, err
		}
		if !s.IsLiteral() {
			return nil, t, p.errorf(t.Pos, "the name of a field in a schema cannot interpolate")
		}
		member.Name = s.Literal()
	default:
		return nil, t, p.errorf(t.Pos, "expected a field of the schema, found %s", t)
	}
	sep := p.cur()
	switch sep.Kind {
	case token.Colon:
	case token.ColonQuestion:
		if member.Name == "" {
			return nil, sep, p.errorf(sep.Pos, "only a field can be optional, so %q follows a name", ":?")
		}
		member.Optional = true
	case token.DoubleColon:
		return nil, sep, p.errorf(sep.Pos, "a schema has no hidden fields; %q is written %q or %q here", "::", ":", ":?")
	default:
		return nil, sep, p.errorf(sep.Pos, "expected %q or %q after the field of a schema, found %s", ":", ":?", sep)
	}
	p.next()
	return member, sep, nil
}

// parseTypeMemberValue parses the type of member, and the default after it
// that only a field may have.
func (p *parser) parseTypeMemberValue(member *ast.TypeMember) error {
	typ, err := p.parseType()
	if err != nil {
		return err
	}
	member.Type = typ
	if !p.at(token.Assign) || !p.sameLine() {
		return nil
	}
	if member.Name == "" {
		return p.errorf(p.cur().Pos, "only a field can have a default")
	}
	p.next()
	member.Default, err = p.parseExpr()
	return err
}

// parseType parses a type: alternatives separated by "|", and the "if" of
// a check that the value has to pass as well.
func (p *parser) parseType() (ast.Node, error) {
	first, err := p.parseTypeAtom()
	if err != nil {
		return nil, err
	}
	typ := first
	if p.at(token.Pipe) && p.sameLine() {
		union := &ast.TypeUnion{Base: ast.At(first.Pos()), Alts: []ast.Node{first}}
		for p.at(token.Pipe) && p.sameLine() {
			p.next()
			alt, err := p.parseTypeAtom()
			if err != nil {
				return nil, err
			}
			union.Alts = append(union.Alts, alt)
		}
		typ = union
	}
	if !p.atKeyword(token.KeywordIf) || !p.sameLine() {
		return typ, nil
	}
	p.next()
	pred, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ast.TypeCheck{Base: ast.At(typ.Pos()), Type: typ, Pred: pred}, nil
}

// parseTypeAtom parses a type that is not a union: a name, a literal, a
// sequence type or a mapping type in braces.
func (p *parser) parseTypeAtom() (ast.Node, error) {
	t := p.cur()
	switch t.Kind {
	case token.Ident:
		switch t.Lit {
		case token.KeywordNull, token.KeywordTrue, token.KeywordFalse:
			v, err := p.parseIdent()
			if err != nil {
				return nil, err
			}
			return &ast.TypeLiteral{Base: ast.At(t.Pos), Value: v}, nil
		}
		if token.IsReserved(t.Lit) || token.IsKeyword(t.Lit) {
			return nil, p.errorf(t.Pos, "expected a type, found %q", t.Lit)
		}
		module, name, err := p.parseSchemaPath()
		if err != nil {
			return nil, err
		}
		return &ast.TypeRef{Base: ast.At(t.Pos), Module: module, Name: name}, nil
	case token.String:
		p.next()
		s, err := p.buildString(t)
		if err != nil {
			return nil, err
		}
		if !s.IsLiteral() {
			return nil, p.errorf(t.Pos, "a string written as a type cannot interpolate")
		}
		return &ast.TypeLiteral{Base: ast.At(t.Pos), Value: s}, nil
	case token.Int, token.Float:
		v, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.TypeLiteral{Base: ast.At(t.Pos), Value: v}, nil
	case token.LBracket:
		p.next()
		item, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if !p.at(token.RBracket) {
			return nil, p.errorf(p.cur().Pos, "expected %q to close a sequence type, found %s", "]", p.cur())
		}
		p.next()
		return &ast.TypeSequence{Base: ast.At(t.Pos), Item: item}, nil
	case token.LBrace:
		return p.parseFlowTypeMapping()
	}
	return nil, p.errorf(t.Pos, "expected a type, found %s", t)
}

// parseSchemaPath parses "Name" or "module.Name", the way a type and a tag
// name a schema.
func (p *parser) parseSchemaPath() (module, name string, err error) {
	first := p.next()
	if !p.at(token.Dots) || !p.adjacent() {
		return "", first.Lit, nil
	}
	dots := p.next()
	if dots.Lit != "." || !p.at(token.Ident) || !p.adjacent() {
		return "", "", p.errorf(dots.Pos, "expected the name of a schema after %q", first.Lit+".")
	}
	second := p.next()
	if p.at(token.Dots) && p.adjacent() {
		return "", "", p.errorf(p.cur().Pos, "a schema is named %q, or %q for one that an imported module declares",
			"Name", "module.Name")
	}
	return first.Lit, second.Lit, nil
}

// parseTagHead parses the "!Name" or "!module.Name" of a tag, leaving the
// value it applies to for the caller. It answers the last token of the
// name as well, which is where a value below the tag is measured from.
func (p *parser) parseTagHead() (*ast.Tag, token.Token, error) {
	bang := p.next()
	t := p.cur()
	if token.IsReserved(t.Lit) || token.IsKeyword(t.Lit) {
		return nil, t, p.errorf(t.Pos, "%q cannot name a schema; to negate a value, %q is written %q",
			t.Lit, "!", token.KeywordNot)
	}
	module, name, err := p.parseSchemaPath()
	if err != nil {
		return nil, t, err
	}
	return &ast.Tag{Base: ast.At(bang.Pos), Module: module, Name: name}, p.toks[p.i-1], nil
}

// parseTag parses a tag written where an operand belongs, which applies to
// the expression after it on the same line.
func (p *parser) parseTag() (ast.Node, error) {
	tag, _, err := p.parseTagHead()
	if err != nil {
		return nil, err
	}
	switch {
	case p.atDocBoundary() || !p.sameLine(),
		p.at(token.Comma), p.at(token.RBracket), p.at(token.RBrace), p.at(token.RParen):
		return nil, p.errorf(p.prevEnd, "expected a value after %q", "!"+tagName(tag))
	}
	if tag.X, err = p.parseExpr(); err != nil {
		return nil, err
	}
	return tag, nil
}

// parseBlockTag parses a tag that a block value starts with. A tag that ends
// its line applies to the block below it, which below parses given the last
// token of the tag; any other applies to the rest of its line.
func (p *parser) parseBlockTag(below func(name token.Token) (ast.Node, error)) (ast.Node, error) {
	tag, last, err := p.parseTagHead()
	if err != nil {
		return nil, err
	}
	if p.atDocBoundary() || !p.sameLine() {
		if tag.X, err = below(last); err != nil {
			return nil, err
		}
		return tag, nil
	}
	if tag.X, err = p.parseExpr(); err != nil {
		return nil, err
	}
	if !p.atDocBoundary() && p.sameLine() {
		return nil, p.errorf(p.cur().Pos, "unexpected %s after value; strings must be quoted", p.cur())
	}
	return tag, nil
}

func tagName(t *ast.Tag) string {
	if t.Module == "" {
		return t.Name
	}
	return t.Module + "." + t.Name
}
