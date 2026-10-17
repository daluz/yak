// Package parser builds an abstract syntax tree from yak source.
//
// Block structure is resolved by comparing token columns rather than by
// consuming INDENT/DEDENT tokens, because YAML permits a block sequence to
// begin at the same column as the mapping key that owns it.
package parser

import (
	"fmt"
	"strings"

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
	toks, comments, err := lexer.Lex(file, src)
	if err != nil {
		return nil, err
	}
	p := &parser{file: file, toks: toks, comments: newCommentSet(comments)}
	return p.parseStream()
}

type parser struct {
	file string
	toks []token.Token
	i    int
	// prevEnd is where the most recently consumed token ended, used to
	// require that references are written without internal whitespace.
	prevEnd token.Pos

	comments *commentSet
}

// takeHead claims the comments written above the current token for the node
// about to be parsed.
func (p *parser) takeHead() []string {
	return p.comments.head(p.i)
}

// takeLine claims the comment trailing a node that began at token start and
// ends at the current position.
func (p *parser) takeLine(start int) string {
	return p.comments.line(start, p.i)
}

func (p *parser) cur() token.Token { return p.toks[p.i] }

func (p *parser) at(k token.Kind) bool { return p.cur().Kind == k }

// atKeyword reports whether the current token is the given contextual
// keyword. One followed by a colon is a mapping key instead, which is what
// keeps names such as "for" and "else" usable as keys.
func (p *parser) atKeyword(word string) bool {
	if !p.at(token.Ident) || p.cur().Lit != word {
		return false
	}
	return !p.peek().Kind.IsSeparator()
}

// peek returns the token after the current one, which is EOF again at the
// end of the input.
func (p *parser) peek() token.Token {
	return p.toks[min(p.i+1, len(p.toks)-1)]
}

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

// sameLine reports whether the current token is on the line the previous one
// ended on.
func (p *parser) sameLine() bool {
	return p.cur().Line() == p.prevEnd.Line
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
		body, err := p.parseDocumentBody()
		if err != nil {
			return nil, err
		}
		st.Docs = append(st.Docs, &ast.Document{Base: ast.At(pos), Body: body})
		if !p.atDocBoundary() {
			return nil, p.errorf(p.cur().Pos, "unexpected %s at document level", p.cur())
		}
		attachFoot(body, p.takeHead())
	}
	if len(st.Docs) == 0 {
		pos := token.Pos{File: p.file, Line: 1, Col: 1}
		st.Docs = append(st.Docs, &ast.Document{Base: ast.At(pos), Body: &ast.Null{Base: ast.At(pos)}})
	}
	return st, nil
}

// attachFoot gives the comments left at the end of a document to the last
// node that is rendered, which is where they come out again.
func attachFoot(body ast.Node, foot []string) {
	if len(foot) == 0 {
		return
	}
	switch t := body.(type) {
	case *ast.Let:
		attachFoot(t.Body, foot)
	case *ast.Mapping:
		for i := len(t.Entries) - 1; i >= 0; i-- {
			if !t.Entries[i].Hidden {
				t.Entries[i].Foot = foot
				return
			}
		}
	case *ast.Sequence:
		if n := len(t.Items); n > 0 {
			t.Items[n-1].Foot = foot
		}
	}
}

// parseDocumentBody parses the node a document holds. Unlike a block nested
// inside one, a document may hold nothing but bindings, and is then null.
func (p *parser) parseDocumentBody() (ast.Node, error) {
	if p.atStatement() {
		return p.parseLetScope(p.cur().Col(), true)
	}
	return p.parseBlockNode()
}

// parseBlockNode parses a node in block context. The node's own extent is
// fixed by the column of its first token: a mapping ends at the first key that
// is less indented, and a sequence at the first dash that is.
func (p *parser) parseBlockNode() (ast.Node, error) {
	t := p.cur()
	if t.Kind == token.Dash {
		return p.parseBlockSequence(t.Col())
	}
	if p.atStatement() {
		return p.parseLetScope(t.Col(), false)
	}
	if p.looksLikeEntry() {
		return p.parseBlockMapping(t.Col())
	}
	return p.parseExpr()
}

// parseLetScope parses a block that opens with one or more let or import
// statements. Bindings in front of a mapping become that mapping's own, so
// that they read the same as bindings written between its entries; anything
// else is wrapped in a scope of its own.
//
// bare allows the bindings to stand on their own, with no value after them,
// which is how a document made of nothing but bindings becomes null.
func (p *parser) parseLetScope(col int, bare bool) (ast.Node, error) {
	pos := p.cur().Pos
	what := p.cur().Lit
	var binds []*ast.Binding
	for p.atStatement() && p.cur().Col() == col {
		declared, err := p.parseStatement(col)
		if err != nil {
			return nil, err
		}
		binds = append(binds, declared...)
	}
	if bare && p.atDocBoundary() {
		return &ast.Let{Base: ast.At(pos), Binds: binds, Body: &ast.Null{Base: ast.At(pos)}}, nil
	}
	if p.atDocBoundary() || p.cur().Col() < col {
		if what == token.KeywordImport {
			return nil, p.errorf(pos, "an %q must be followed by a value in the same block", what)
		}
		return nil, p.errorf(pos, "a %q binding must be followed by a value in the same block", what)
	}
	if p.cur().Col() > col {
		return nil, p.errorf(p.cur().Pos, "unexpected indentation: expected a value at column %d", col)
	}
	if p.looksLikeEntry() {
		m, err := p.parseBlockMapping(col)
		if err != nil {
			return nil, err
		}
		m.Binds = append(binds, m.Binds...)
		return m, nil
	}
	body, err := p.parseBlockNode()
	if err != nil {
		return nil, err
	}
	return &ast.Let{Base: ast.At(pos), Binds: binds, Body: body}, nil
}

func (p *parser) parseBlockMapping(col int) (*ast.Mapping, error) {
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
		if p.atStatement() {
			declared, err := p.parseStatement(col)
			if err != nil {
				return nil, err
			}
			m.Binds = append(m.Binds, declared...)
			continue
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
	start := p.i
	head := p.takeHead()
	entry, err := p.parseEntryHead()
	if err != nil {
		return nil, err
	}
	colon := p.next()
	value, err := p.parseEntryValue(col, colon)
	if err != nil {
		return nil, err
	}
	entry.Comments = ast.Comments{Head: head, Line: p.takeLine(start)}
	entry.Value = value
	return entry, nil
}

// parseEntryHead parses everything an entry has before its value: a key, or
// the "<<" that stands where a key belongs, and the separator after it. The
// separator is left as the current token, since a block entry needs it to
// find its value.
func (p *parser) parseEntryHead() (*ast.Entry, error) {
	entry := &ast.Entry{KeyPos: p.cur().Pos}
	if p.at(token.Merge) {
		p.next()
		entry.Merge = true
	} else {
		key, computed, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		entry.Key, entry.Computed = key, computed
	}
	switch p.cur().Kind {
	case token.Colon:
	case token.DoubleColon:
		entry.Hidden = true
	case token.ColonQuestion:
		entry.HideNull = true
	default:
		return nil, p.errorf(p.cur().Pos, "expected %q, %q or %q after mapping key, found %s", ":", "::", ":?", p.cur())
	}
	return entry, nil
}

// parseEntryValue parses the value that follows a mapping key, which is either
// on the same line as the colon or in a more indented block below it.
func (p *parser) parseEntryValue(parentCol int, colon token.Token) (ast.Node, error) {
	if p.atDocBoundary() {
		return &ast.Null{Base: ast.At(colon.Pos)}, nil
	}
	t := p.cur()
	if t.Line() == colon.Line() {
		v, err := p.parseExpr()
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

// atStatement reports whether a let or import statement starts at the
// current token. Both keywords are reserved, so they can never be anything
// else here.
func (p *parser) atStatement() bool {
	if !p.at(token.Ident) {
		return false
	}
	return p.cur().Lit == token.KeywordLet || p.cur().Lit == token.KeywordImport
}

// parseStatement parses one "let name = value" binding or one
// "import module", or a braced block of either.
//
// The comments written on a binding are claimed by the binding itself rather
// than left for whatever follows it, since a binding renders nothing of its
// own and its comments travel to wherever it is read back.
func (p *parser) parseStatement(col int) ([]*ast.Binding, error) {
	start := p.i
	head := p.takeHead()
	kw := p.next()
	// A colon here means the line was meant to be an entry keyed "let".
	if p.cur().Kind.IsSeparator() {
		return nil, p.errorf(kw.Pos, "%q is a reserved word and must be quoted to be used as a key", kw.Lit)
	}
	if kw.Lit == token.KeywordImport {
		return p.parseImport(start, head)
	}
	if p.at(token.LBrace) {
		return p.parseBlock(token.KeywordLet, head, p.parseBlockBinding)
	}
	bh, err := p.parseBindHead()
	if err != nil {
		return nil, err
	}
	// The value follows the "=" exactly as a mapping value follows its
	// colon, so a binding may hold an indented block.
	value, err := p.parseEntryValue(col, bh.assign)
	if err != nil {
		if bh.pattern != nil && p.at(token.Comma) && p.cur().Line() == bh.assign.Line() {
			return nil, p.errorf(p.cur().Pos, "a pattern destructures one sequence, so the values go in brackets, as in %q", "a, b = [1, 2]")
		}
		return nil, err
	}
	bind := bh.bind(value)
	bind.Comments = ast.Comments{Head: head, Line: p.takeLine(start)}
	return []*ast.Binding{bind}, nil
}

// parseBlock parses the braced form of a "let" or "import" statement, named
// by kw. Bindings are written one per line, or separated by commas when
// several share a line, and item parses each one. head holds the comments
// written above the block, which belong to the first binding in it.
func (p *parser) parseBlock(kw string, head []string, item func() (*ast.Binding, error)) ([]*ast.Binding, error) {
	open := p.next()
	var binds []*ast.Binding
	// separated records that the previous binding ran to the end of its line
	// and so needs a comma before another one may follow it.
	separated := true
	for {
		if p.at(token.EOF) {
			return nil, p.errorf(open.Pos, "unterminated %q block: missing %q", kw, "}")
		}
		if p.at(token.RBrace) {
			p.next()
			if len(binds) == 0 {
				return nil, p.errorf(open.Pos, "the %q block must declare at least one binding", kw)
			}
			return binds, nil
		}
		if !separated {
			return nil, p.errorf(p.cur().Pos, "expected %q or a line break between bindings, found %s", ",", p.cur())
		}
		start := p.i
		above := p.takeHead()
		if len(binds) == 0 {
			above = append(head, above...)
		}
		bind, err := item()
		if err != nil {
			return nil, err
		}
		bind.Comments = ast.Comments{Head: above, Line: p.takeLine(start)}
		binds = append(binds, bind)
		if p.at(token.Comma) {
			p.next()
			continue
		}
		separated = !p.sameLine()
	}
}

// parseBlockBinding parses one binding of a "let { ... }" block, whose value
// is an expression rather than a block of its own.
func (p *parser) parseBlockBinding() (*ast.Binding, error) {
	bh, err := p.parseBindHead()
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return bh.bind(value), nil
}

// parseImport parses what follows "import": one module, or a braced block of
// them. start is the index of the keyword and head the comments above it.
func (p *parser) parseImport(start int, head []string) ([]*ast.Binding, error) {
	if p.at(token.LBrace) {
		return p.parseBlock(token.KeywordImport, head, p.parseImportSpec)
	}
	if p.atDocBoundary() || !p.sameLine() {
		return nil, p.errorf(p.prevEnd, "expected the path of a module after %q", token.KeywordImport)
	}
	bind, err := p.parseImportSpec()
	if err != nil {
		return nil, err
	}
	if !p.atDocBoundary() && p.sameLine() {
		return nil, p.errorf(p.cur().Pos, "unexpected %s after the path of an %q", p.cur(), token.KeywordImport)
	}
	bind.Comments = ast.Comments{Head: head, Line: p.takeLine(start)}
	return []*ast.Binding{bind}, nil
}

// parseImportSpec parses "name = a.b.c" or "a.b.c". A module is bound under
// the name it is given, or else under the last segment of its path.
func (p *parser) parseImportSpec() (*ast.Binding, error) {
	var name token.Token
	if p.at(token.Ident) && p.peek().Kind == token.Assign {
		t, err := p.parseName("module")
		if err != nil {
			return nil, err
		}
		name = t
		p.next()
	}
	if !p.at(token.Ident) {
		return nil, p.errorf(p.cur().Pos, "expected the path of a module, found %s", p.cur())
	}
	first := p.next()
	imp := &ast.Import{Base: ast.At(first.Pos), Path: []string{first.Lit}}
	last := first
	for p.at(token.Dots) && p.adjacent() {
		dots := p.next()
		if dots.Lit != "." {
			return nil, p.errorf(dots.Pos, "%q in the path of a module; segments are separated by a single %q", dots.Lit, ".")
		}
		if !p.at(token.Ident) || !p.adjacent() {
			return nil, p.errorf(p.cur().Pos, "expected a segment of the module path after %q, found %s", ".", p.cur())
		}
		last = p.next()
		imp.Path = append(imp.Path, last.Lit)
	}
	if name.Lit == "" {
		if token.IsReserved(last.Lit) || token.IsKeyword(last.Lit) {
			return nil, p.errorf(last.Pos, "%q cannot name a module; bind it to another name with %q",
				last.Lit, "import name = "+strings.Join(imp.Path, "."))
		}
		name = last
	}
	return &ast.Binding{Base: ast.At(name.Pos), Name: name.Lit, Value: imp}, nil
}

// parseName parses the name of a binding or of a parameter, which is an
// identifier that no keyword has already claimed.
func (p *parser) parseName(what string) (token.Token, error) {
	t := p.cur()
	if t.Kind != token.Ident {
		return t, p.errorf(t.Pos, "expected the name of a %s, found %s", what, t)
	}
	if token.IsReserved(t.Lit) {
		return t, p.errorf(t.Pos, "%q is a reserved word and cannot name a %s", t.Lit, what)
	}
	if token.IsKeyword(t.Lit) {
		return t, p.errorf(t.Pos, "%q is a keyword and cannot name a %s", t.Lit, what)
	}
	if t.Lit == ast.Wildcard {
		return t, p.errorf(t.Pos, "%q discards a value and cannot name a %s", t.Lit, what)
	}
	p.next()
	return t, nil
}

// parseSignature parses the parameter list that turns a binding into a
// function. It returns nil when the name is not followed by "(", which is an
// ordinary binding of a value.
func (p *parser) parseSignature(name token.Token) (*ast.Function, error) {
	if !p.at(token.LParen) {
		return nil, nil
	}
	fn := &ast.Function{Base: ast.At(name.Pos), Name: name.Lit}
	if err := p.parseParams(fn); err != nil {
		return nil, err
	}
	return fn, nil
}

// parseParams parses the parameter list of fn, which the current token must
// open. A named function and an anonymous one declare their parameters the
// same way, so both come through here.
func (p *parser) parseParams(fn *ast.Function) error {
	open := p.next()
	for {
		more, err := p.nextInList(open, paramList, len(fn.Params) == 0)
		if !more {
			return err
		}
		param, err := p.parseParam(fn)
		if err != nil {
			return err
		}
		fn.Params = append(fn.Params, param)
	}
}

// list describes a comma-separated list in brackets, for nextInList.
type list struct {
	closer token.Kind
	// close is how closer is written.
	close string
	// name is what the list is called, and in what an unexpected token is
	// reported as being found in.
	name, in string
}

var (
	paramList   = list{closer: token.RParen, close: ")", name: "parameter list", in: "a parameter list"}
	argList     = list{closer: token.RParen, close: ")", name: "argument list", in: "an argument list"}
	flowSeqList = list{closer: token.RBracket, close: "]", name: "flow sequence", in: "flow sequence"}
	flowMapList = list{closer: token.RBrace, close: "}", name: "flow mapping", in: "flow mapping"}
)

// nextInList moves to the next item of the list l that open opened, and
// reports whether there is one. It consumes the comma after the item before
// it, which first says there is none of, or the bracket that closes the list.
func (p *parser) nextInList(open token.Token, l list, first bool) (bool, error) {
	separated := first
	if !first && p.at(token.Comma) {
		p.next()
		separated = true
	}
	switch {
	case p.at(token.EOF):
		return false, p.errorf(open.Pos, "unterminated %s: missing %q", l.name, l.close)
	case p.at(l.closer):
		p.next()
		return false, nil
	case !separated:
		return false, p.errorf(p.cur().Pos, "expected %q or %q in %s, found %s", ",", l.close, l.in, p.cur())
	}
	return true, nil
}

// parseParam parses one parameter of fn, along with the default value that a
// call may leave out.
func (p *parser) parseParam(fn *ast.Function) (*ast.Param, error) {
	var param *ast.Param
	if p.at(token.LBracket) {
		pat, err := p.parseNamingPattern("parameter")
		if err != nil {
			return nil, err
		}
		param = &ast.Param{Base: pat.Base, Pattern: pat}
	} else {
		name, err := p.parseName("parameter")
		if err != nil {
			return nil, err
		}
		param = &ast.Param{Base: ast.At(name.Pos), Name: name.Lit}
	}
	if err := p.checkParamNames(fn.Params, param); err != nil {
		return nil, err
	}
	var err error
	if p.at(token.Assign) {
		p.next()
		if param.Default, err = p.parseExpr(); err != nil {
			return nil, err
		}
	}
	return param, nil
}

// checkParamNames rejects a parameter that binds a name one declared before
// it already binds, whether either of them is a pattern or not.
func (p *parser) checkParamNames(prev []*ast.Param, param *ast.Param) error {
	taken := map[string]bool{}
	for _, q := range prev {
		eachParamName(q, func(name string, _ token.Pos) { taken[name] = true })
	}
	var err error
	eachParamName(param, func(name string, pos token.Pos) {
		if taken[name] && err == nil {
			err = p.errorf(pos, "parameter %q is declared twice", name)
		}
	})
	return err
}

func eachParamName(param *ast.Param, f func(string, token.Pos)) {
	if param.Pattern == nil {
		f(param.Name, param.Pos())
		return
	}
	param.Pattern.Walk(func(name *ast.Pattern, _ []int) { f(name.Name, name.Pos()) })
}

// bindHead is everything a binding has before its value: its name, the
// function that its parameter list declared, and the "=" that ends it. A
// binding that destructures has a pattern instead of a name and a function.
type bindHead struct {
	name    token.Token
	fn      *ast.Function
	pattern *ast.Pattern
	assign  token.Token
}

// parseBindHead parses the name of a binding through to its "=", which is
// written the same way whether the binding stands on its own or inside a
// "let" block.
func (p *parser) parseBindHead() (bindHead, error) {
	if p.at(token.LBracket) || (p.at(token.Ident) && p.peek().Kind == token.Comma) {
		return p.parsePatternHead()
	}
	name, err := p.parseName("binding")
	if err != nil {
		return bindHead{}, err
	}
	fn, err := p.parseSignature(name)
	if err != nil {
		return bindHead{}, err
	}
	assign, err := p.expectAssign()
	if err != nil {
		return bindHead{}, err
	}
	return bindHead{name: name, fn: fn, assign: assign}, nil
}

// parsePatternHead parses the "[a, b] =" of a binding that destructures, or
// the "a, b =" that leaves out the brackets of the outermost pattern.
func (p *parser) parsePatternHead() (bindHead, error) {
	pat, err := p.parsePatternList("binding")
	if err != nil {
		return bindHead{}, err
	}
	if err := p.checkNaming(pat, "binding"); err != nil {
		return bindHead{}, err
	}
	assign, err := p.expectAssign()
	if err != nil {
		return bindHead{}, err
	}
	return bindHead{pattern: pat, assign: assign}, nil
}

// parseNamingPattern parses a bracketed pattern that has to bind a name, as
// a parameter or a binding written as one does.
func (p *parser) parseNamingPattern(what string) (*ast.Pattern, error) {
	pat, err := p.parsePattern(what)
	if err != nil {
		return nil, err
	}
	return pat, p.checkNaming(pat, what)
}

// checkNaming rejects a pattern that binds a name twice or binds none. One of
// nothing but wildcards would never be read, and so never be checked against
// its value, which makes it a mistake rather than a test.
func (p *parser) checkNaming(pat *ast.Pattern, what string) error {
	if err := p.checkPattern(pat); err != nil {
		return err
	}
	named := false
	pat.Walk(func(*ast.Pattern, []int) { named = true })
	if !named {
		return p.errorf(pat.Pos(), "the pattern of a %s has to bind at least one name", what)
	}
	return nil
}

// bind builds the binding of the head to value, wrapping it in the function
// that its parameter list declared.
func (h bindHead) bind(value ast.Node) *ast.Binding {
	if h.pattern != nil {
		return &ast.Binding{Base: h.pattern.Base, Pattern: h.pattern, Value: value}
	}
	if h.fn != nil {
		h.fn.Body = value
		value = h.fn
	}
	return &ast.Binding{Base: ast.At(h.name.Pos), Name: h.name.Lit, Value: value}
}

func (p *parser) expectAssign() (token.Token, error) {
	if !p.at(token.Assign) {
		return p.cur(), p.errorf(p.cur().Pos, "expected %q after the name of a binding, found %s", "=", p.cur())
	}
	return p.next(), nil
}

func (p *parser) parseBlockSequence(col int) (ast.Node, error) {
	s := &ast.Sequence{Base: ast.At(p.cur().Pos)}
	for p.cur().Kind == token.Dash && p.cur().Col() == col {
		start := p.i
		head := p.takeHead()
		dash := p.next()
		value, err := p.parseSequenceItem(dash)
		if err != nil {
			return nil, err
		}
		s.Items = append(s.Items, &ast.Item{
			Comments: ast.Comments{Head: head, Line: p.takeLine(start)},
			Value:    value,
		})
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
	return p.toks[j].Kind.IsSeparator()
}

// keyEnd returns the index just past a key-shaped run of tokens starting at i,
// or -1 if no key can start there.
func (p *parser) keyEnd(i int) int {
	switch p.toks[i].Kind {
	case token.Ident, token.String, token.Int, token.Float, token.Merge:
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
