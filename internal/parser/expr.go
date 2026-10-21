package parser

import (
	"slices"
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/lexer"
	"github.com/daluz/yak/internal/token"
)

// parseExpr parses an expression. A conditional or a match binds looser than
// every operator, so it is recognised before anything else.
func (p *parser) parseExpr() (ast.Node, error) {
	switch {
	case p.atKeyword(token.KeywordIf):
		return p.parseIf()
	case p.atKeyword(token.KeywordMatch):
		return p.parseMatch()
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
	for p.at(token.Coalesce) && p.sameLine() {
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
	if p.sameLine() && (p.at(token.Dash) || p.atSignedNumber()) {
		return nil, p.errorf(p.cur().Pos, "a subtraction needs whitespace on both sides of its %q", "-")
	}
	// An arrow left over here follows something that was not a parameter
	// list, which is how a function written without parentheses reads.
	if p.at(token.Arrow) && p.sameLine() {
		return nil, p.errorf(p.cur().Pos,
			"an anonymous function needs its parameters in parentheses, as in %q", "(x) => x")
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
	{token.Lt, token.Le, token.Gt, token.Ge, token.In, token.NotIn},
	{token.Plus, token.Merge, token.Dash},
	{token.Star, token.Slash, token.Percent},
}

// notLevel is the level a "not" is read at. It binds looser than every
// comparison and tighter than "and", so "not a == b" negates the comparison.
const notLevel = 2

func (p *parser) parseBinary(level int) (ast.Node, error) {
	if level == len(precedence) {
		return p.parseUnary()
	}
	if level == notLevel && p.atKeyword(token.KeywordNot) {
		t := p.next()
		x, err := p.parseBinary(level)
		if err != nil {
			return nil, err
		}
		return &ast.Unary{Base: ast.At(t.Pos), Op: token.Not, X: x}, nil
	}
	x, err := p.parseBinary(level + 1)
	if err != nil {
		return nil, err
	}
	for p.atBinaryOp(level) {
		kind := p.operator()
		op := p.next()
		if kind == token.NotIn {
			p.next()
		}
		y, err := p.parseBinary(level + 1)
		if err != nil {
			return nil, err
		}
		x = &ast.Binary{Base: ast.At(x.Pos()), Op: kind, OpPos: op.Pos, X: x, Y: y}
	}
	return x, nil
}

// wordOperators maps the keywords that are written as operators to the kinds
// they stand for.
var wordOperators = map[string]token.Kind{
	token.KeywordAnd: token.And,
	token.KeywordOr:  token.Or,
	token.KeywordIn:  token.In,
}

// operator answers the kind of the current token, reading a word operator as
// the operator it is. A "not" followed by "in" is the one operator spelled
// with two words.
func (p *parser) operator() token.Kind {
	t := p.cur()
	if k, ok := wordOperators[t.Lit]; ok && p.atKeyword(t.Lit) {
		return k
	}
	if p.atKeyword(token.KeywordNot) {
		if next := p.peek(); next.Kind == token.Ident && next.Lit == token.KeywordIn {
			return token.NotIn
		}
	}
	return t.Kind
}

// atBinaryOp reports whether an operator of the given level stands at the
// current position, ready to extend the operand already parsed.
func (p *parser) atBinaryOp(level int) bool {
	if !slices.Contains(precedence[level], p.operator()) || !p.sameLine() {
		return false
	}
	// A "-" may appear inside an identifier and may negate what follows it,
	// so a subtraction is the one written with whitespace on both sides:
	// "a-b" is a name, "a -b" is two operands, and "a - b" subtracts.
	if p.at(token.Dash) {
		return !p.adjacent() && p.peek().Pos != p.cur().End
	}
	return true
}

func (p *parser) parseUnary() (ast.Node, error) {
	if p.at(token.Dash) {
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

// parseMatch parses "match x case T then y ... else z". Like a conditional,
// its values are full expressions, so a trailing "else" belongs to the
// innermost match that is still open.
func (p *parser) parseMatch() (ast.Node, error) {
	kw := p.next()
	x, err := p.parseCoalesce()
	if err != nil {
		return nil, err
	}
	n := &ast.Match{Base: ast.At(kw.Pos), X: x}
	for p.atKeyword(token.KeywordCase) {
		c, err := p.parseCase()
		if err != nil {
			return nil, err
		}
		n.Cases = append(n.Cases, c)
	}
	if len(n.Cases) == 0 {
		return nil, p.errorf(p.cur().Pos, "expected %q after the value of a %q, found %s", "case", "match", p.cur())
	}
	if p.atKeyword(token.KeywordElse) {
		p.next()
		if n.Else, err = p.parseExpr(); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// parseCase parses "case T as name then value", whose "as name" may be left
// out. The items of a tuple T may bind names of their own.
func (p *parser) parseCase() (*ast.Case, error) {
	kw := p.next()
	p.binding = true
	typ, err := p.parseType()
	p.binding = false
	if err != nil {
		return nil, err
	}
	c := &ast.Case{Base: ast.At(kw.Pos), Type: typ}
	names := []token.Token{}
	if p.atKeyword(token.KeywordAs) {
		p.next()
		name, err := p.parseName("case")
		if err != nil {
			return nil, err
		}
		c.Name = name.Lit
		names = append(names, name)
	}
	if err := p.checkCaseNames(typ, names); err != nil {
		return nil, err
	}
	if !p.atKeyword(token.KeywordThen) {
		return nil, p.errorf(p.cur().Pos, "expected %q after the type of a %q, found %s", "then", "case", p.cur())
	}
	p.next()
	if c.Value, err = p.parseExpr(); err != nil {
		return nil, err
	}
	return c, nil
}

// checkCaseNames rejects a case that binds a name twice, among the names the
// items of typ bind and names.
func (p *parser) checkCaseNames(typ ast.Node, names []token.Token) error {
	seen := map[string]bool{}
	var walk func(ast.Node) error
	walk = func(t ast.Node) error {
		switch t := t.(type) {
		case *ast.TypeBind:
			if seen[t.Name] {
				return p.errorf(t.Pos(), "%q is bound twice in one case", t.Name)
			}
			seen[t.Name] = true
			return walk(t.Type)
		case *ast.TypeCheck:
			return walk(t.Type)
		case *ast.TypeTuple:
			for _, item := range t.Items {
				if err := walk(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(typ); err != nil {
		return err
	}
	for _, name := range names {
		if seen[name.Lit] {
			return p.errorf(name.Pos, "%q is bound twice in one case", name.Lit)
		}
	}
	return nil
}

// parseLoop parses a "for pattern in source" clause of a comprehension,
// along with its optional "if" filter.
func (p *parser) parseLoop(pos token.Pos) (ast.Loop, error) {
	var loop ast.Loop
	p.next()
	if t := p.cur(); t.Kind != token.Ident && t.Kind != token.LBracket {
		return loop, p.errorf(t.Pos, "expected the name of the loop variable after %q, found %s", "for", t)
	}
	pat, err := p.parsePatternList("loop variable")
	if err != nil {
		return loop, err
	}
	if err := p.checkPattern(pat); err != nil {
		return loop, err
	}
	if !p.atKeyword(token.KeywordIn) {
		return loop, p.errorf(p.cur().Pos, "expected %q after the loop variable, found %s", "in", p.cur())
	}
	p.next()
	source, err := p.parseExpr()
	if err != nil {
		return loop, err
	}
	loop = ast.Loop{Base: ast.At(pos), Pattern: pat, Source: source}
	if p.atKeyword(token.KeywordIf) {
		p.next()
		if loop.Filter, err = p.parseExpr(); err != nil {
			return loop, err
		}
	}
	return loop, nil
}

// parsePatternList parses a pattern, or several separated by commas, which
// are one list pattern written without its brackets.
func (p *parser) parsePatternList(what string) (*ast.Pattern, error) {
	pat, err := p.parsePattern(what)
	if err != nil || !p.at(token.Comma) {
		return pat, err
	}
	pat = &ast.Pattern{Base: pat.Base, Items: []*ast.Pattern{pat}}
	for p.at(token.Comma) {
		p.next()
		item, err := p.parsePattern(what)
		if err != nil {
			return nil, err
		}
		pat.Items = append(pat.Items, item)
	}
	return pat, nil
}

// parsePattern parses a name, the wildcard, or a bracketed list of patterns.
// what says what the names are for, which a misplaced token is reported as.
func (p *parser) parsePattern(what string) (*ast.Pattern, error) {
	t := p.cur()
	if t.Kind == token.Ident && t.Lit == ast.Wildcard {
		p.next()
		return &ast.Pattern{Base: ast.At(t.Pos), Name: ast.Wildcard}, nil
	}
	if t.Kind != token.LBracket {
		name, err := p.parseName(what)
		if err != nil {
			return nil, err
		}
		return &ast.Pattern{Base: ast.At(name.Pos), Name: name.Lit}, nil
	}
	p.next()
	pat := &ast.Pattern{Base: ast.At(t.Pos), Items: []*ast.Pattern{}}
	for {
		if p.at(token.RBracket) {
			if len(pat.Items) == 0 {
				return nil, p.errorf(t.Pos, "a pattern needs at least one item")
			}
			return nil, p.errorf(p.cur().Pos, "expected a pattern after %q, found %s", ",", p.cur())
		}
		item, err := p.parsePattern(what)
		if err != nil {
			return nil, err
		}
		pat.Items = append(pat.Items, item)
		if p.at(token.RBracket) {
			p.next()
			return pat, nil
		}
		if !p.at(token.Comma) {
			return nil, p.errorf(p.cur().Pos, "expected %q or %q in a pattern, found %s", ",", "]", p.cur())
		}
		p.next()
	}
}

// checkPattern rejects a pattern that binds the same name twice. The
// wildcard binds nothing, so it may be written any number of times.
func (p *parser) checkPattern(pat *ast.Pattern) error {
	seen := map[string]bool{}
	var err error
	pat.Walk(func(name *ast.Pattern, _ []int) {
		if seen[name.Name] && err == nil {
			err = p.errorf(name.Pos(), "%q is bound twice in one pattern", name.Name)
		}
		seen[name.Name] = true
	})
	return err
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
		return p.parseBuiltin()

	case token.LBracket:
		return p.parseFlowSequence()

	case token.LBrace:
		return p.parseFlowMapping()

	case token.LParen:
		if p.atLambda() {
			return p.parseLambda()
		}
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
		return nil, p.errorf(t.Pos, "anchors and aliases are not supported in yak; use a let binding instead")

	case token.Bang:
		return p.parseTag()

	default:
		return nil, p.errorf(t.Pos, "expected a value, found %s", t)
	}
}

// atLambda reports whether the "(" at the current position opens the
// parameter list of an anonymous function rather than a parenthesised
// expression. Nothing before the matching ")" tells the two apart, so this
// looks for the "=>" after it. A ")" can only appear inside a string or
// balanced against a "(", so counting parentheses finds the right one.
func (p *parser) atLambda() bool {
	depth := 0
	for i := p.i; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen:
			depth++
		case token.RParen:
			depth--
			if depth == 0 {
				return p.toks[i+1].Kind == token.Arrow
			}
		case token.EOF:
			return false
		}
	}
	return false
}

// parseLambda parses "(params) => body", a function with no name. The
// parameters are the ones a binding declares, defaults included, and the body
// is an expression: a function whose body needs a block of its own has a name
// to hang it on.
func (p *parser) parseLambda() (ast.Node, error) {
	fn := &ast.Function{Base: ast.At(p.cur().Pos)}
	if err := p.parseParams(fn); err != nil {
		return nil, err
	}
	// atLambda already found the arrow that follows the parameters.
	p.next()
	body, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	fn.Body = body
	return fn, nil
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
	case token.KeywordLet:
		return nil, p.errorf(t.Pos, "a %q binding is a statement of its own, not a value", "let")
	case token.KeywordImport:
		return nil, p.errorf(t.Pos, "an %q is a statement of its own, not a value", "import")
	case token.KeywordSchema:
		return nil, p.errorf(t.Pos, "a %q is a statement of its own, not a value; a value is checked against one with %q",
			"schema", "!Name")
	case token.KeywordIf:
		// An "if" is looser than every operator, so one written where an
		// operand belongs has to say which way it groups.
		return nil, p.errorf(t.Pos, "an %q cannot be used as an operand here; wrap it in parentheses", "if")
	case token.KeywordMatch:
		return nil, p.errorf(t.Pos, "a %q cannot be used as an operand here; wrap it in parentheses", "match")
	case token.KeywordNot:
		// A "not" is looser than every comparison and every arithmetic
		// operator, so it cannot stand on the right of one unwrapped.
		return nil, p.errorf(t.Pos, "a %q cannot be used as an operand here; wrap it in parentheses", "not")
	}
	if t.Lit == ast.Wildcard {
		return nil, p.errorf(t.Pos, "%q discards a value and cannot be read", t.Lit)
	}
	if token.IsKeyword(t.Lit) {
		return nil, p.errorf(t.Pos, "%q is a keyword and cannot be used as a value; quote it to make it a string", t.Lit)
	}
	// Whether a name such as "no" is a mistyped boolean or a binding cannot
	// be decided here; the evaluator knows what is in scope and says so.
	return &ast.Ident{Base: ast.At(t.Pos), Name: t.Lit}, nil
}

// builtinVariables names every "$name" variable, so that a misspelled one is
// answered with the list of the real ones.
const builtinVariables = `"$root", "$context", "$ctx", "$yak", "$filepath", "$workdir"`

// parseBuiltin parses a "$name" variable. Some of them are a sigil written
// out in full and mean exactly what the sigil does, so "$root" is "$" and
// "$context" and "$ctx" are "$$". The rest describe the run rather than the
// document.
func (p *parser) parseBuiltin() (ast.Node, error) {
	t := p.next()
	switch t.Lit {
	case "root":
		return &ast.Root{Base: ast.At(t.Pos)}, nil
	case "context", "ctx":
		return &ast.Context{Base: ast.At(t.Pos)}, nil
	case "yak":
		return &ast.Yak{Base: ast.At(t.Pos)}, nil
	case "filepath":
		return &ast.FilePath{Base: ast.At(t.Pos)}, nil
	case "workdir":
		return &ast.Workdir{Base: ast.At(t.Pos)}, nil
	}
	return nil, p.errorf(t.Pos, "unknown builtin variable %q; the ones that exist are %s",
		"$"+t.Lit, builtinVariables)
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
		more, err := p.nextInList(open, argList, len(args) == 0)
		if !more {
			return args, err
		}
		var arg ast.Arg
		switch {
		case p.at(token.Ident) && p.peek().Kind == token.Assign:
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
	}
}

func (p *parser) parseFlowSequence() (ast.Node, error) {
	open := p.next()
	s := &ast.Sequence{Base: ast.At(open.Pos), Flow: true}
	for {
		more, err := p.nextInList(open, flowSeqList, len(s.Items) == 0)
		if !more {
			return s, err
		}
		item, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if len(s.Items) == 0 && p.atKeyword(token.KeywordFor) {
			return p.parseSeqComp(open, item)
		}
		s.Items = append(s.Items, &ast.Item{Value: item})
	}
}

func (p *parser) parseFlowMapping() (ast.Node, error) {
	open := p.next()
	m := &ast.Mapping{Base: ast.At(open.Pos), Flow: true}
	for {
		more, err := p.nextInList(open, flowMapList, len(m.Entries) == 0)
		if !more {
			return m, err
		}
		entry, err := p.parseEntryHead()
		if err != nil {
			return nil, err
		}
		p.next()
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		entry.Value = value
		if len(m.Entries) == 0 && p.atKeyword(token.KeywordFor) {
			return p.parseMapComp(open, entry)
		}
		m.Entries = append(m.Entries, entry)
	}
}

// parseLoops parses the "for" clauses of a comprehension, the current token
// being the first "for".
func (p *parser) parseLoops() ([]ast.Loop, error) {
	var loops []ast.Loop
	for p.atKeyword(token.KeywordFor) {
		loop, err := p.parseLoop(p.cur().Pos)
		if err != nil {
			return nil, err
		}
		loops = append(loops, loop)
	}
	return loops, nil
}

// parseSeqComp parses the rest of "[item for name in source]", with item
// already parsed and the current token being "for".
func (p *parser) parseSeqComp(open token.Token, item ast.Node) (ast.Node, error) {
	loops, err := p.parseLoops()
	if err != nil {
		return nil, err
	}
	if !p.at(token.RBracket) {
		return nil, p.errorf(p.cur().Pos, "expected %q to close a comprehension, found %s", "]", p.cur())
	}
	p.next()
	return &ast.SeqComp{Base: ast.At(open.Pos), Loops: loops, Item: item}, nil
}

// parseMapComp parses the rest of "{key: value for name in source}", with the
// entry already parsed and the current token being "for".
func (p *parser) parseMapComp(open token.Token, entry *ast.Entry) (ast.Node, error) {
	if entry.Merge {
		return nil, p.errorf(entry.KeyPos, "a comprehension needs an entry of its own, so it cannot start with %q", "<<")
	}
	loops, err := p.parseLoops()
	if err != nil {
		return nil, err
	}
	if !p.at(token.RBrace) {
		return nil, p.errorf(p.cur().Pos, "expected %q to close a comprehension, found %s", "}", p.cur())
	}
	p.next()
	return &ast.MapComp{Base: ast.At(open.Pos), Loops: loops, Entry: entry}, nil
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
