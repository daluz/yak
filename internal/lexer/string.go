package lexer

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/daluz/yak/internal/token"
)

// decoded accumulates the characters of a string literal together with the
// source position of every byte, so that interpolations can be reported at
// their real location even after escapes and block folding have been applied.
type decoded struct {
	buf []byte
	pos []token.Pos
	// escaped marks bytes produced by an escape sequence. Such bytes never
	// start an interpolation, which is what makes `\u007b` a literal brace.
	escaped []bool
	start   token.Pos
}

func newDecoded(start token.Pos) *decoded {
	return &decoded{start: start}
}

func (d *decoded) addByte(b byte, p token.Pos, esc bool) {
	d.buf = append(d.buf, b)
	d.pos = append(d.pos, p)
	d.escaped = append(d.escaped, esc)
}

func (d *decoded) addString(s string, p token.Pos, esc bool) {
	for i := 0; i < len(s); i++ {
		d.addByte(s[i], p, esc)
	}
}

// addText appends text that lies on a single source line, advancing the
// recorded column for every byte so that interpolations inside block scalars
// report their real position.
func (d *decoded) addText(s string, start token.Pos) {
	for i := 0; i < len(s); i++ {
		d.addByte(s[i], token.Pos{File: start.File, Line: start.Line, Col: start.Col + i}, false)
	}
}

func (d *decoded) posAt(i int) token.Pos {
	if i < len(d.pos) {
		return d.pos[i]
	}
	return d.start
}

func (l *lexer) scanQuoted(raw bool) error {
	return l.scanQuotedAt(l.pos(), raw)
}

func (l *lexer) scanQuotedAt(start token.Pos, raw bool) error {
	quote := l.peek()
	l.advance()
	d := newDecoded(start)
	closed := false
	for !l.eof() {
		p := l.pos()
		c := l.peek()
		switch {
		case c == quote:
			l.advance()
			if quote == '\'' && l.peek() == '\'' {
				l.advance()
				d.addByte('\'', p, true)
				continue
			}
			closed = true
		case c == '\n':
			l.advance()
			blanks := 0
			for !l.eof() {
				for l.peek() == ' ' || l.peek() == '\t' || l.peek() == '\r' {
					l.advance()
				}
				if l.peek() != '\n' {
					break
				}
				l.advance()
				blanks++
			}
			if blanks > 0 {
				d.addString(strings.Repeat("\n", blanks), p, false)
			} else {
				d.addByte(' ', p, false)
			}
			continue
		case c == '\\' && quote == '"' && !raw:
			if err := l.scanEscape(d); err != nil {
				return err
			}
			continue
		case c == '{' && !raw && l.peekAt(1) == '{':
			// The "{{" escape for a literal brace: copy it through for the
			// splitter to collapse.
			l.copyByte(d)
			l.copyByte(d)
			continue
		case c == '{' && !raw:
			// Copy the interpolation verbatim so that a quote inside it, as
			// in "{ $$.m["k"] }", does not end the enclosing string. The
			// splitter re-reads the same region afterwards.
			if err := l.copyInterpolation(d); err != nil {
				return err
			}
			continue
		default:
			d.addByte(l.advance(), p, false)
			continue
		}
		break
	}
	if !closed {
		return l.errorf(start, "unterminated string literal")
	}
	return l.emitString(start, d, raw)
}

// copyByte moves one source byte into the decoded buffer unchanged.
func (l *lexer) copyByte(d *decoded) byte {
	p := l.pos()
	c := l.advance()
	d.addByte(c, p, false)
	return c
}

// copyInterpolation copies a "{ ... }" region verbatim, tracking brace depth
// and nested string literals so that the enclosing string is not terminated
// early by a quote belonging to the expression.
func (l *lexer) copyInterpolation(d *decoded) error {
	start := l.pos()
	l.copyByte(d) // {
	depth := 1
	for !l.eof() {
		switch l.peek() {
		case '{':
			depth++
			l.copyByte(d)
		case '}':
			depth--
			l.copyByte(d)
			if depth == 0 {
				return nil
			}
		case '"', '\'':
			if err := l.copyNestedQuoted(d); err != nil {
				return err
			}
		default:
			l.copyByte(d)
		}
	}
	return l.errorf(start, "unterminated string interpolation: missing %q; write %q for a literal brace", "}", "{{")
}

func (l *lexer) copyNestedQuoted(d *decoded) error {
	start := l.pos()
	quote := l.copyByte(d)
	for !l.eof() {
		c := l.peek()
		if c == '\\' && quote == '"' {
			l.copyByte(d)
			if l.eof() {
				break
			}
			l.copyByte(d)
			continue
		}
		l.copyByte(d)
		if c == quote {
			if quote == '\'' && l.peek() == '\'' {
				l.copyByte(d)
				continue
			}
			return nil
		}
	}
	return l.errorf(start, "unterminated string literal inside an interpolation; write %q for a literal brace", "{{")
}

func (l *lexer) scanEscape(d *decoded) error {
	p := l.pos()
	l.advance() // backslash
	if l.eof() {
		return l.errorf(p, "unterminated escape sequence")
	}
	c := l.advance()
	switch c {
	case '0':
		d.addByte(0, p, true)
	case 'a':
		d.addByte(0x07, p, true)
	case 'b':
		d.addByte(0x08, p, true)
	case 't', '\t':
		d.addByte('\t', p, true)
	case 'n':
		d.addByte('\n', p, true)
	case 'v':
		d.addByte(0x0b, p, true)
	case 'f':
		d.addByte(0x0c, p, true)
	case 'r':
		d.addByte('\r', p, true)
	case 'e':
		d.addByte(0x1b, p, true)
	case ' ':
		d.addByte(' ', p, true)
	case '"', '\'', '\\', '/':
		d.addByte(c, p, true)
	case 'N':
		d.addRune(0x85, p)
	case '_':
		d.addRune(0xa0, p)
	case 'L':
		d.addRune(0x2028, p)
	case 'P':
		d.addRune(0x2029, p)
	case 'x':
		return l.scanHexEscape(d, p, 2)
	case 'u':
		return l.scanHexEscape(d, p, 4)
	case 'U':
		return l.scanHexEscape(d, p, 8)
	case '\n':
		// Line continuation: drop the break and the next line's indentation.
		for l.peek() == ' ' || l.peek() == '\t' {
			l.advance()
		}
	default:
		return l.errorf(p, "unknown escape sequence %q", `\`+string(rune(c)))
	}
	return nil
}

func (d *decoded) addRune(r rune, p token.Pos) {
	var b [utf8.UTFMax]byte
	n := utf8.EncodeRune(b[:], r)
	for i := 0; i < n; i++ {
		d.addByte(b[i], p, true)
	}
}

func (l *lexer) scanHexEscape(d *decoded, p token.Pos, n int) error {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if l.eof() || !isHex(l.peek()) {
			return l.errorf(p, "escape sequence needs %d hexadecimal digits", n)
		}
		sb.WriteByte(l.advance())
	}
	v, err := strconv.ParseUint(sb.String(), 16, 32)
	if err != nil {
		return l.errorf(p, "invalid hexadecimal escape %q", sb.String())
	}
	if n == 2 {
		d.addByte(byte(v), p, true)
		return nil
	}
	d.addRune(rune(v), p)
	return nil
}

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

type blockLine struct {
	text   string
	pos    token.Pos
	indent int
	blank  bool
}

func (l *lexer) scanBlockScalar(start token.Pos, raw bool) error {
	if !l.block {
		return l.errorf(start, "block scalars are not allowed here")
	}
	headerIndent := l.lineIndent()
	style := l.advance() // | or >

	chomp := byte(0)
	explicit := 0
	for i := 0; i < 2; i++ {
		switch c := l.peek(); {
		case c == '-' || c == '+':
			if chomp != 0 {
				return l.errorf(l.pos(), "duplicate chomping indicator")
			}
			chomp = l.advance()
		case c >= '1' && c <= '9':
			if explicit != 0 {
				return l.errorf(l.pos(), "duplicate indentation indicator")
			}
			explicit = int(l.advance() - '0')
		}
	}
	for l.peek() == ' ' || l.peek() == '\t' || l.peek() == '\r' {
		l.advance()
	}
	if l.peek() == '#' {
		for !l.eof() && l.peek() != '\n' {
			l.advance()
		}
	}
	if !l.eof() && l.peek() != '\n' {
		return l.errorf(l.pos(), "unexpected content after block scalar header; the block body must start on the next line")
	}
	if !l.eof() {
		l.advance()
	}

	contentIndent := -1
	if explicit > 0 {
		contentIndent = headerIndent + explicit
	}
	var lines []blockLine
	for !l.eof() {
		offSave, lineSave, colSave, bolSave := l.off, l.line, l.col, l.bol
		bl := l.readBlockLine()
		if !bl.blank {
			if contentIndent < 0 {
				contentIndent = bl.indent
				if contentIndent <= headerIndent {
					l.off, l.line, l.col, l.bol = offSave, lineSave, colSave, bolSave
					break
				}
			}
			if bl.indent < contentIndent {
				l.off, l.line, l.col, l.bol = offSave, lineSave, colSave, bolSave
				break
			}
		}
		lines = append(lines, bl)
	}
	if contentIndent < 0 {
		contentIndent = headerIndent + 1
	}

	// Drop trailing blank lines from the body; chomping decides how many of
	// their line breaks survive.
	last := -1
	for i, bl := range lines {
		if !bl.blank {
			last = i
		}
	}
	trailingBlanks := 0
	if last >= 0 {
		trailingBlanks = len(lines) - 1 - last
	} else {
		lines = nil
	}

	d := newDecoded(start)
	body := lines[:last+1]
	for i := range body {
		body[i] = stripIndent(body[i], contentIndent)
	}
	if style == '|' {
		for i, bl := range body {
			if i > 0 {
				d.addByte('\n', bl.pos, false)
			}
			d.addText(bl.text, bl.pos)
		}
	} else {
		l.foldLines(d, body, contentIndent)
	}
	if len(body) > 0 {
		switch chomp {
		case '-':
		case '+':
			for i := 0; i <= trailingBlanks; i++ {
				d.addByte('\n', body[len(body)-1].pos, false)
			}
		default:
			d.addByte('\n', body[len(body)-1].pos, false)
		}
	}
	return l.emitString(start, d, raw)
}

// foldLines implements folded ("greater than") block scalars: a single line
// break becomes a space, blank lines become line breaks, and lines indented
// beyond the block indentation keep their breaks verbatim.
func (l *lexer) foldLines(d *decoded, body []blockLine, contentIndent int) {
	blanks := 0
	first := true
	prevMore := false
	for _, bl := range body {
		if bl.blank {
			blanks++
			continue
		}
		more := bl.indent > contentIndent
		switch {
		case first:
			first = false
		case blanks > 0:
			d.addString(strings.Repeat("\n", blanks), bl.pos, false)
		case more || prevMore:
			d.addByte('\n', bl.pos, false)
		default:
			d.addByte(' ', bl.pos, false)
		}
		d.addText(bl.text, bl.pos)
		blanks = 0
		prevMore = more
	}
}

func stripIndent(bl blockLine, indent int) blockLine {
	if bl.blank {
		return blockLine{text: "", pos: bl.pos, indent: indent, blank: true}
	}
	text := bl.text
	if len(text) >= indent {
		text = text[indent:]
	} else {
		text = strings.TrimLeft(text, " ")
	}
	return blockLine{
		text:   text,
		pos:    token.Pos{File: bl.pos.File, Line: bl.pos.Line, Col: indent + 1},
		indent: bl.indent,
		blank:  false,
	}
}

// readBlockLine consumes one physical line, including its terminator.
func (l *lexer) readBlockLine() blockLine {
	pos := l.pos()
	var sb strings.Builder
	for !l.eof() && l.peek() != '\n' {
		sb.WriteByte(l.advance())
	}
	if !l.eof() {
		l.advance()
	}
	text := strings.TrimRight(sb.String(), "\r")
	trimmed := strings.TrimLeft(text, " ")
	return blockLine{
		text:   text,
		pos:    pos,
		indent: len(text) - len(trimmed),
		blank:  strings.TrimSpace(text) == "",
	}
}

// lineIndent returns the number of leading spaces on the line the lexer is
// currently positioned in.
func (l *lexer) lineIndent() int {
	start := l.off
	for start > 0 && l.src[start-1] != '\n' {
		start--
	}
	n := 0
	for start+n < len(l.src) && l.src[start+n] == ' ' {
		n++
	}
	return n
}

func (l *lexer) emitString(start token.Pos, d *decoded, raw bool) error {
	if raw {
		l.emit(token.Token{Kind: token.String, Lit: string(d.buf), Pos: start})
		return nil
	}
	chunks, err := splitInterpolations(d)
	if err != nil {
		return err
	}
	switch {
	case len(chunks) == 0:
		l.emit(token.Token{Kind: token.String, Lit: "", Pos: start})
	case len(chunks) == 1 && !chunks[0].IsExpr:
		l.emit(token.Token{Kind: token.String, Lit: chunks[0].Text, Pos: start})
	default:
		l.emit(token.Token{Kind: token.String, Chunks: chunks, Pos: start})
	}
	return nil
}

// splitInterpolations breaks decoded string content into literal and {...}
// expression chunks. A doubled brace is the escape for a literal one: "{{"
// yields "{" and "}}" yields "}". Doubling is the only escape, so it works in
// every interpolating string form, including block scalars and single-quoted
// strings where backslash escapes do not exist.
func splitInterpolations(d *decoded) ([]token.Chunk, error) {
	var (
		chunks []token.Chunk
		lit    strings.Builder
		litPos token.Pos
	)
	flush := func() {
		if lit.Len() > 0 {
			chunks = append(chunks, token.Chunk{Text: lit.String(), Pos: litPos})
			lit.Reset()
		}
	}
	// The buffer holds raw bytes of UTF-8 text, so literal runs are copied
	// byte by byte rather than converted through string(byte).
	addLitByte := func(b byte, p token.Pos) {
		if lit.Len() == 0 {
			litPos = p
		}
		lit.WriteByte(b)
	}

	buf := d.buf
	doubled := func(i int) bool {
		return i+1 < len(buf) && buf[i+1] == buf[i] && !d.escaped[i+1]
	}

	for i := 0; i < len(buf); {
		if (buf[i] == '{' || buf[i] == '}') && !d.escaped[i] && doubled(i) {
			addLitByte(buf[i], d.posAt(i))
			i += 2
			continue
		}
		if buf[i] == '{' && !d.escaped[i] {
			end, err := matchBrace(buf, i+1)
			if err != nil {
				return nil, &Error{Pos: d.posAt(i), Msg: err.Error()}
			}
			flush()
			chunks = append(chunks, token.Chunk{
				Text:   string(buf[i+1 : end]),
				IsExpr: true,
				Pos:    d.posAt(i + 1),
			})
			i = end + 1
			continue
		}
		addLitByte(buf[i], d.posAt(i))
		i++
	}
	flush()
	return chunks, nil
}

// matchBrace finds the "}" closing an interpolation that starts at from,
// skipping over nested braces and over quoted strings inside the expression.
func matchBrace(buf []byte, from int) (int, error) {
	depth := 1
	for i := from; i < len(buf); i++ {
		switch buf[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		case '"':
			i++
			for i < len(buf) && buf[i] != '"' {
				if buf[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(buf) {
				return 0, errUnterminatedInterp
			}
		case '\'':
			i++
			for i < len(buf) {
				if buf[i] == '\'' {
					if i+1 < len(buf) && buf[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			if i >= len(buf) {
				return 0, errUnterminatedInterp
			}
		}
	}
	return 0, errUnterminatedInterp
}

var errUnterminatedInterp = interpError("unterminated string interpolation: missing \"}\"; write \"{{\" for a literal brace")

type interpError string

func (e interpError) Error() string { return string(e) }
