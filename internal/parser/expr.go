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

// parseExpr parses an expression. A conditional binds looser than every
// operator, so it is recognised before anything else.
func (p *parser) parseExpr() (ast.Node, error) {
	if p.atKeyword(token.KeywordIf) {
		return p.parseIf()
	}
	return p.parseCoalesce()
}

// parseCoalesce parses the "??" level, which binds looser than the boolean
// and comparison operators.
//
// The right hand side may spill onto later lines, but the operator itself
// must stay on the line its left operand ended on, so that a block mapping is
// never silently continued by the line below it. Every binary operator
// follows that rule.
func (p *parser) parseCoalesce() (ast.Node, error) {
	x, err := p.parseBinary(0)
	if err != nil {
		return nil, err
	}
	for p.at(token.Coalesce) && p.cur().Line() == p.prevEnd.Line {
		p.next()
		y, err := p.parseBinary(0)
		if err != nil {
			return nil, err
		}
		x = &ast.Coalesce{Base: ast.At(x.Pos()), X: x, Y: y}
	}
	// A "-" left over here was written without the whitespace a subtraction
	// needs, and so was a number the lexer read with its sign attached.
	// Either way the operator was meant, so say so rather than leave the
	// caller to report two values in a row.
	if p.cur().Line() == p.prevEnd.Line && (p.at(token.Dash) || p.atSignedNumber()) {
		return nil, p.errorf(p.cur().Pos, "a subtraction needs whitespace on both sides of its %q", "-")
	}
	return x, nil
}

// atSignedNumber reports whether the current token is a number whose sign the
// lexer folded into it, which is how "a -2" reads.
func (p *parser) atSignedNumber() bool {
	t := p.cur()
	return (t.Kind == token.Int || t.Kind == token.Float) && strings.HasPrefix(t.Lit, "-")
}

// precedence lists the binary operators by level, loosest first. Every level
// is left associative.
var precedence = [][]token.Kind{
	{token.Or},
	{token.And},
	{token.Eq, token.Ne},
	{token.Lt, token.Le, token.Gt, token.Ge},
	{token.Plus, token.Dash},
	{token.Star, token.Slash, token.Percent},
}

func (p *parser) parseBinary(level int) (ast.Node, error) {
	if level == len(precedence) {
		return p.parseUnary()
	}
	x, err := p.parseBinary(level + 1)
	if err != nil {
		return nil, err
	}
	for p.atBinaryOp(level) {
		op := p.next()
		y, err := p.parseBinary(level + 1)
		if err != nil {
			return nil, err
		}
		x = &ast.Binary{Base: ast.At(x.Pos()), Op: op.Kind, OpPos: op.Pos, X: x, Y: y}
	}
	return x, nil
}

// atBinaryOp reports whether an operator of the given level stands at the
// current position, ready to extend the operand already parsed.
func (p *parser) atBinaryOp(level int) bool {
	if !p.atAny(precedence[level]) || p.cur().Line() != p.prevEnd.Line {
		return false
	}
	// A "-" may appear inside an identifier and may negate what follows it,
	// so a subtraction is the one written with whitespace on both sides:
	// "a-b" is a name, "a -b" is two operands, and "a - b" subtracts.
	if p.at(token.Dash) {
		return !p.adjacent() && p.toks[p.i+1].Pos != p.cur().End
	}
	return true
}

func (p *parser) atAny(kinds []token.Kind) bool {
	for _, k := range kinds {
		if p.at(k) {
			return true
		}
	}
	return false
}

func (p *parser) parseUnary() (ast.Node, error) {
	if p.at(token.Not) || p.at(token.Dash) {
		t := p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ast.Unary{Base: ast.At(t.Pos), Op: t.Kind, X: x}, nil
	}
	return p.parseOperand()
}

// parseIf parses "if cond then x else y". The branches are full expressions,
// so "else if" chains without parentheses and a trailing "else" belongs to
// the innermost conditional that is still open.
func (p *parser) parseIf() (ast.Node, error) {
	kw := p.next()
	cond, err := p.parseCoalesce()
	if err != nil {
		return nil, err
	}
	if !p.atKeyword(token.KeywordThen) {
		return nil, p.errorf(p.cur().Pos, "expected %q after the condition of an %q, found %s",
			"then", "if", p.cur())
	}
	p.next()
	then, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	n := &ast.If{Base: ast.At(kw.Pos), Cond: cond, Then: then}
	if p.atKeyword(token.KeywordElse) {
		p.next()
		if n.Else, err = p.parseExpr(); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// parseLoop parses the "for name in source" clause that turns a flow
// collection into a comprehension, along with its optional "if" filter.
func (p *parser) parseLoop(pos token.Pos) (ast.Loop, error) {
	var loop ast.Loop
	p.next()
	name := p.cur()
	switch {
	case name.Kind != token.Ident:
		return loop, p.errorf(name.Pos, "expected the name of the loop variable after %q, found %s", "for", name)
	case token.IsReserved(name.Lit) || token.IsKeyword(name.Lit):
		return loop, p.errorf(name.Pos, "%q is a keyword and cannot name a loop variable", name.Lit)
	}
	p.next()
	if !p.atKeyword(token.KeywordIn) {
		return loop, p.errorf(p.cur().Pos, "expected %q after the loop variable, found %s", "in", p.cur())
	}
	p.next()
	source, err := p.parseExpr()
	if err != nil {
		return loop, err
	}
	loop = ast.Loop{Base: ast.At(pos), Var: name.Lit, VarPos: name.Pos, Source: source}
	if p.atKeyword(token.KeywordIf) {
		p.next()
		if loop.Filter, err = p.parseExpr(); err != nil {
			return loop, err
		}
	}
	return loop, nil
}

func (p *parser) parseOperand() (ast.Node, error) {
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
		return p.parseSpecial()

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

	case token.Star:
		// A "*" multiplies two operands, so one standing where a value
		// belongs is an alias rather than an operator.
		return nil, p.errorf(t.Pos, "anchors and aliases are not supported in yak; use a local variable instead")

	default:
		return nil, p.errorf(t.Pos, "expected a value, found %s", t)
	}
}

// unsupportedKeywords maps the statement keywords that are planned but not
// yet implemented to the message shown when they are used.
var unsupportedKeywords = map[string]string{
	token.KeywordImport: `"import" is not implemented yet`,
	token.KeywordSchema: `"schema" is not implemented yet`,
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
	case token.KeywordLocal:
		return nil, p.errorf(t.Pos, "a %q binding is a statement of its own, not a value", "local")
	case token.KeywordIf:
		// An "if" is looser than every operator, so one written where an
		// operand belongs has to say which way it groups.
		return nil, p.errorf(t.Pos, "an %q cannot be used as an operand here; wrap it in parentheses", "if")
	}
	if msg, ok := unsupportedKeywords[t.Lit]; ok {
		return nil, p.errorf(t.Pos, "%s", msg)
	}
	if token.IsKeyword(t.Lit) {
		return nil, p.errorf(t.Pos, "%q is a keyword and cannot be used as a value; quote it to make it a string", t.Lit)
	}
	// Whether a name such as "no" is a mistyped boolean or a binding cannot
	// be decided here; the evaluator knows what is in scope and says so.
	return &ast.Ident{Base: ast.At(t.Pos), Name: t.Lit}, nil
}

// specialVariables names every "$name" variable, so that a misspelled one is
// answered with the list of the real ones.
const specialVariables = `"$root", "$self", "$context", "$yak"`

// parseSpecial parses a "$name" variable. Three of them are a sigil written
// out in full and mean exactly what the sigil does, so "$root" is "$",
// "$self" is "." and "$context" is "$$". "$yak" describes the run rather
// than the document.
func (p *parser) parseSpecial() (ast.Node, error) {
	t := p.next()
	switch t.Lit {
	case "root":
		return &ast.Root{Base: ast.At(t.Pos)}, nil
	case "self":
		return &ast.Self{Base: ast.At(t.Pos), Src: "$self"}, nil
	case "context":
		return &ast.Context{Base: ast.At(t.Pos)}, nil
	case "yak":
		return &ast.Yak{Base: ast.At(t.Pos)}, nil
	case "super":
		return nil, p.errorf(t.Pos, "%q is not implemented yet", "$super")
	}
	return nil, p.errorf(t.Pos, "unknown special variable %q; the ones that exist are %s",
		"$"+t.Lit, specialVariables)
}

// parseSelf parses a leading run of dots. A run of n dots walks n-1 mappings
// outwards, so "." is the current mapping and ".." is its parent. A field name
// written immediately after the dots binds to that mapping.
func (p *parser) parseSelf() (ast.Node, error) {
	dots := p.next()
	self := &ast.Self{Base: ast.At(dots.Pos), Up: len(dots.Lit) - 1, Src: dots.Lit}
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

// parsePostfix applies field accesses, index accesses and calls. Each must be
// written flush against the expression it applies to, so "$.b .c" is a mistake
// rather than a silently accepted "$.b.c".
func (p *parser) parsePostfix(x ast.Node) (ast.Node, error) {
	for {
		if !p.adjacent() {
			return x, nil
		}
		optional := p.at(token.Question)
		if optional {
			p.next()
		}
		t := p.cur()
		switch {
		case t.Kind == token.Dots:
			if len(t.Lit) > 1 {
				return nil, p.errorf(t.Pos, "%q may only begin a reference, not continue one", t.Lit)
			}
			dots := p.next()
			name, ok := p.adjacentFieldName(dots)
			if !ok {
				return nil, p.errorf(p.cur().Pos, "expected a field name after %q", ".")
			}
			x = &ast.Field{Base: ast.At(x.Pos()), X: x, Name: name, Optional: optional}
		case t.Kind == token.LBracket:
			p.next()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.at(token.RBracket) {
				return nil, p.errorf(p.cur().Pos, "expected %q to close an index, found %s", "]", p.cur())
			}
			p.next()
			x = &ast.Index{Base: ast.At(x.Pos()), X: x, Index: idx, Optional: optional}
		case optional:
			return nil, p.errorf(t.Pos, "expected %q or %q after %q, found %s", ".", "[", "?", t)
		case t.Kind == token.LParen:
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			x = &ast.Call{Base: ast.At(x.Pos()), Fn: x, Args: args}
		default:
			return x, nil
		}
	}
}

// parseArgs parses the argument list of a call. A named argument is written
// "name = value", the way a parameter declares its default, and the
// positional ones all come before the first of them.
func (p *parser) parseArgs() ([]ast.Arg, error) {
	open := p.next()
	var args []ast.Arg
	named := false
	for {
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated argument list: missing %q", ")")
		}
		if p.at(token.RParen) {
			p.next()
			return args, nil
		}
		var arg ast.Arg
		switch {
		case p.at(token.Ident) && p.toks[p.i+1].Kind == token.Assign:
			name := p.next()
			p.next()
			arg.Name, arg.NamePos = name.Lit, name.Pos
			named = true
		case named:
			return nil, p.errorf(p.cur().Pos, "a positional argument cannot follow a named one")
		}
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		arg.Value = value
		args = append(args, arg)
		if p.at(token.Comma) {
			p.next()
			continue
		}
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated argument list: missing %q", ")")
		}
		if !p.at(token.RParen) {
			return nil, p.errorf(p.cur().Pos, "expected %q or %q in an argument list, found %s", ",", ")", p.cur())
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
		if len(s.Items) == 0 && p.atKeyword(token.KeywordFor) {
			return p.parseSeqComp(open, item)
		}
		s.Items = append(s.Items, &ast.Item{Value: item})
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
		var hidden, hideNull bool
		switch p.cur().Kind {
		case token.Colon:
		case token.DoubleColon:
			hidden = true
		case token.ColonQuestion:
			hideNull = true
		default:
			return nil, p.errorf(p.cur().Pos, "expected %q, %q or %q after mapping key, found %s", ":", "::", ":?", p.cur())
		}
		p.next()
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		entry := &ast.Entry{
			Key: key, Computed: computed, Hidden: hidden, HideNull: hideNull, Value: value, KeyPos: keyPos,
		}
		if len(m.Entries) == 0 && p.atKeyword(token.KeywordFor) {
			return p.parseMapComp(open, entry)
		}
		m.Entries = append(m.Entries, entry)
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

// parseSeqComp parses the rest of "[item for name in source]", with item
// already parsed and the current token being "for".
func (p *parser) parseSeqComp(open token.Token, item ast.Node) (ast.Node, error) {
	loop, err := p.parseLoop(open.Pos)
	if err != nil {
		return nil, err
	}
	if !p.at(token.RBracket) {
		return nil, p.errorf(p.cur().Pos, "expected %q to close a comprehension, found %s", "]", p.cur())
	}
	p.next()
	return &ast.SeqComp{Loop: loop, Item: item}, nil
}

// parseMapComp parses the rest of "{key: value for name in source}", with the
// entry already parsed and the current token being "for".
func (p *parser) parseMapComp(open token.Token, entry *ast.Entry) (ast.Node, error) {
	loop, err := p.parseLoop(open.Pos)
	if err != nil {
		return nil, err
	}
	if !p.at(token.RBrace) {
		return nil, p.errorf(p.cur().Pos, "expected %q to close a comprehension, found %s", "}", p.cur())
	}
	p.next()
	return &ast.MapComp{Loop: loop, Entry: entry}, nil
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
	sub := &parser{file: p.file, toks: toks, comments: newCommentSet(nil)}
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
