// Package yakfmt rewrites yak source in its canonical layout.
//
// Only the whitespace between tokens changes: every token, string and comment
// keeps its spelling, and every line break stays where it was written. Each
// level of indentation becomes two spaces, the spaces within a line follow
// fixed rules, runs of blank lines become one, and the trailing comments of
// consecutive lines are aligned. The one exception is the TypeNames a caller
// asks for, which respell the built-in types and their conversions.
package yakfmt

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/daluz/yak/internal/lexer"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/token"
)

// indentStep is the width of one level of indentation.
const indentStep = 2

// Source formats a yak file. A file that does not parse is an error, and so
// is one whose formatting would read back differently, which would be a bug.
func Source(file string, src []byte, names TypeNames) ([]byte, error) {
	before, err := parser.Parse(file, src)
	if err != nil {
		return nil, err
	}
	toks, comments, err := lexer.Lex(file, src)
	if err != nil {
		return nil, err
	}
	renames := respell(before, toks, names)
	p := &printer{src: src, starts: lineStarts(src), levels: []level{{orig: 1}}, renames: renames}
	next := 0
	for i, t := range toks {
		for ; next < len(comments) && comments[next].Next <= i; next++ {
			p.comment(comments[next])
		}
		if t.Kind != token.EOF {
			p.token(t)
		}
	}
	out := []byte(p.finish())

	for i, t := range toks {
		if name, ok := renames[t.Pos]; ok {
			toks[i].Lit = name
		}
	}
	afterToks, afterComments, err := lexer.Lex(file, out)
	if err == nil && slices.EqualFunc(toks, afterToks, sameToken) && slices.EqualFunc(comments, afterComments, sameComment) {
		after, err := parser.Parse(file, out)
		if err == nil && sameTree(reflect.ValueOf(before), reflect.ValueOf(after)) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("%s: formatting would change what the file means, so it was left alone; this is a bug in yak fmt", file)
}

// level maps the column a block was written at to the indentation it is
// given.
type level struct {
	orig   int
	indent int
}

// line is one line of output. Code holds its indentation and tokens, and
// more than one source line when a string spans several.
type line struct {
	code    string
	comment string
	// blank records a blank line written above it.
	blank bool
}

type printer struct {
	src    []byte
	starts []int
	lines  []line
	// levels holds the blocks enclosing the current line, outermost first.
	levels []level
	// prev is the last token written, and prevEnd the source line where the
	// last token or comment ended.
	prev    token.Token
	prevEnd int
	// indicator records that prev is the "-" of a block sequence item.
	indicator bool
	// delta is how far the current line moved from where it was written.
	delta int
	// renames holds the spelling of each token that TypeNames respells.
	renames map[token.Pos]string
}

func (p *printer) token(t token.Token) {
	if len(p.lines) == 0 || t.Pos.Line > p.prevEnd {
		indent := p.indent(t.Pos.Col)
		p.delta = indent - (t.Pos.Col - 1)
		p.newLine(t.Pos.Line, strings.Repeat(" ", indent))
		p.indicator = t.Kind == token.Dash
	} else {
		cur := &p.lines[len(p.lines)-1]
		cur.code += strings.Repeat(" ", gap(p.prev, t, t.Pos.Col-p.prev.End.Col))
		if p.indicator {
			// What follows the "-" of an item is a block of its own, which
			// the lines below it may continue.
			p.levels = append(p.levels, level{orig: t.Pos.Col, indent: utf8.RuneCountInString(cur.code)})
		}
		p.indicator = p.indicator && t.Kind == token.Dash
	}
	cur := &p.lines[len(p.lines)-1]
	text := string(p.src[p.offset(t.Pos):p.offset(t.End)])
	if name, ok := p.renames[t.Pos]; ok {
		text = name
	}
	if strings.Contains(text, "\n") {
		text = reindent(text, p.shift(text, t.Pos.Col))
	}
	cur.code += text
	p.prev = t
	p.prevEnd = t.End.Line
}

func (p *printer) comment(c token.Comment) {
	if !c.OwnLine {
		p.lines[len(p.lines)-1].comment = c.Text
		return
	}
	l := p.enclosing(c.Pos.Col)
	indent := l.indent
	if l.orig < c.Pos.Col {
		indent += indentStep
	}
	p.newLine(c.Pos.Line, strings.Repeat(" ", indent)+c.Text)
	p.prevEnd = c.Pos.Line
}

// enclosing answers the innermost level written at or before col.
func (p *printer) enclosing(col int) level {
	for _, l := range slices.Backward(p.levels) {
		if l.orig <= col {
			return l
		}
	}
	return p.levels[0]
}

// indent answers the indentation of a line whose first token was written at
// col, opening a level for it when it is deeper than the enclosing one.
func (p *printer) indent(col int) int {
	for p.levels[len(p.levels)-1].orig > col {
		p.levels = p.levels[:len(p.levels)-1]
	}
	top := p.levels[len(p.levels)-1]
	if top.orig == col {
		return top.indent
	}
	p.levels = append(p.levels, level{orig: col, indent: top.indent + indentStep})
	return top.indent + indentStep
}

func (p *printer) newLine(srcLine int, code string) {
	p.lines = append(p.lines, line{code: code, blank: len(p.lines) > 0 && srcLine > p.prevEnd+1})
}

// finish joins the lines, aligning the trailing comments of each run of
// consecutive lines that have one.
func (p *printer) finish() string {
	var b strings.Builder
	for i := 0; i < len(p.lines); {
		j, width := i, 0
		for j < len(p.lines) && aligns(p.lines[j]) && (j == i || !p.lines[j].blank) {
			width = max(width, utf8.RuneCountInString(p.lines[j].code))
			j++
		}
		if j == i {
			j, width = i+1, utf8.RuneCountInString(p.lines[i].code)
		}
		for _, l := range p.lines[i:j] {
			if l.blank {
				b.WriteByte('\n')
			}
			b.WriteString(l.code)
			if l.comment != "" {
				b.WriteString(strings.Repeat(" ", width-utf8.RuneCountInString(l.code)+1))
				b.WriteString(l.comment)
			}
			b.WriteByte('\n')
		}
		i = j
	}
	return b.String()
}

func aligns(l line) bool {
	return l.comment != "" && !strings.Contains(l.code, "\n")
}

// spaced lists the operators written with a space on either side.
var spaced = map[token.Kind]bool{
	token.Plus: true, token.Star: true, token.Slash: true, token.Percent: true,
	token.Merge: true, token.Coalesce: true, token.Assign: true, token.Arrow: true, token.Pipe: true,
	token.Eq: true, token.Ne: true, token.Lt: true, token.Le: true, token.Gt: true, token.Ge: true,
}

// gap answers how many spaces go between two tokens on one line that were
// written orig columns apart. Where none of the rules applies, any run of
// spaces becomes one and tokens written flush stay flush, since that is what
// tells "-x" from "- x" and "f(x)" from "f (x)".
func gap(prev, next token.Token, orig int) int {
	switch {
	case next.Kind == token.Comma, next.Kind.IsSeparator(), next.Kind == token.RBracket, next.Kind == token.RParen,
		prev.Kind == token.LBracket, prev.Kind == token.LParen:
		return 0
	case prev.Kind == token.Comma, prev.Kind.IsSeparator():
		return 1
	case prev.Kind == token.LBrace, next.Kind == token.RBrace:
		return min(orig, 1)
	case next.Kind == token.Star && prev.Kind == token.Ident && prev.Lit == token.KeywordSchema:
		return 0
	case spaced[prev.Kind], spaced[next.Kind]:
		return 1
	}
	return min(orig, 1)
}

// shift answers how far to move the lines after the first of a string that
// spans lines, written at col: so that they start one level deeper than the
// block holding the string, or, for a block scalar whose header gives its
// indentation, as far as the line holding the header moved.
func (p *printer) shift(text string, col int) int {
	header, body, _ := strings.Cut(text, "\n")
	header = strings.TrimPrefix(header, "r")
	if strings.HasPrefix(header, "|") || strings.HasPrefix(header, ">") {
		if strings.ContainsAny(header[1:min(len(header), 3)], "123456789") {
			return p.delta
		}
	}
	indent := p.enclosing(col - 1).indent
	for l := range strings.SplitSeq(body, "\n") {
		if strings.TrimSpace(l) != "" {
			return indent + indentStep - (len(l) - len(strings.TrimLeft(l, " ")))
		}
	}
	return p.delta
}

// reindent moves the lines after the first of a token's text by delta
// columns.
func reindent(text string, delta int) string {
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		l := strings.TrimSuffix(lines[i], "\r")
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " "))
		lines[i] = strings.Repeat(" ", max(n+delta, 0)) + l[n:]
	}
	return strings.Join(lines, "\n")
}

func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (p *printer) offset(pos token.Pos) int {
	return p.starts[pos.Line-1] + pos.Col - 1
}

func sameToken(a, b token.Token) bool {
	return a.Kind == b.Kind && a.Lit == b.Lit
}

func sameComment(a, b token.Comment) bool {
	return a.Text == b.Text && a.OwnLine == b.OwnLine && a.Next == b.Next
}

var posType = reflect.TypeFor[token.Pos]()

// sameTree reports whether two syntax trees are the same but for the
// positions their nodes were read from.
func sameTree(a, b reflect.Value) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return sameTree(a.Elem(), b.Elem())
	case reflect.Struct:
		if a.Type() == posType {
			return true
		}
		for i := range a.NumField() {
			if !sameTree(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if a.Len() != b.Len() {
			return false
		}
		for i := range a.Len() {
			if !sameTree(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	}
	return a.Equal(b)
}
