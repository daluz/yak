package render

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/eval"
)

// commentStyle is how a bracketed format writes a comment, or dropComments
// when the format has nowhere to put one.
type commentStyle int

const (
	dropComments commentStyle = iota
	// hashComments writes "# text", as YAML and yak do.
	hashComments
	// slashComments writes "// text", as the JSON dialects that allow
	// comments do.
	slashComments
)

// bracketStyle is the punctuation of one bracketed format. KYAML and the JSON
// family share a writer and differ only in these knobs.
type bracketStyle struct {
	format Format
	indent int
	// header precedes every document.
	header string
	// single marks a format that holds exactly one document.
	single bool
	// bareKeys leaves a key unquoted when it cannot be mistaken for
	// anything else.
	bareKeys bool
	// trailingComma writes a comma after the last entry of a collection.
	trailingComma bool
	comments      commentStyle
	// cuddle opens a nested bracket at the end of the line before it,
	// turning "[\n  {" into "[{".
	cuddle bool
	// foldStrings breaks a multi-line string over as many output lines,
	// joined by YAML flow folds, instead of writing one long escaped line.
	foldStrings bool
	// compact writes a whole document on one line, without indentation.
	compact bool
	// floats names the non-finite floats, which JSON cannot write at all.
	floats floatSpelling
}

func bracketStyleFor(opts Options) bracketStyle {
	s := bracketStyle{format: opts.Format, indent: opts.Indent}
	switch opts.Format {
	case FormatKYAML:
		// A KYAML document opens with "---" so that it cannot be mistaken
		// for JSON, which also starts with "{".
		s.header = "---\n"
		s.bareKeys = true
		s.trailingComma = true
		s.comments = hashComments
		s.cuddle = true
		s.foldStrings = true
		s.floats = yamlFloats
	case FormatJSON:
		s.single = true
	case FormatJSONC:
		s.single = true
		s.comments = slashComments
	case FormatJWCC:
		s.single = true
		s.trailingComma = true
		s.comments = slashComments
	case FormatJSONL:
		s.compact = true
	}
	return s
}

func encodeBracketed(docs []eval.Value, opts Options) ([]byte, error) {
	style := bracketStyleFor(opts)
	if style.single {
		if err := singleDocument(style.format, docs); err != nil {
			return nil, err
		}
	}

	w := &bracketWriter{style: style}
	for _, doc := range docs {
		w.buf.WriteString(style.header)
		if err := w.value(doc, 0); err != nil {
			return nil, err
		}
		w.buf.WriteByte('\n')
	}
	return w.buf.Bytes(), nil
}

type bracketWriter struct {
	buf   bytes.Buffer
	style bracketStyle
}

// value writes v. depth is the nesting level of the line the value starts on,
// which is also where its closing bracket lands.
func (w *bracketWriter) value(v eval.Value, depth int) error {
	switch t := v.(type) {
	case eval.Null:
		w.buf.WriteString("null")
	case eval.Bool:
		w.buf.WriteString(strconv.FormatBool(bool(t)))
	case eval.Int:
		w.buf.WriteString(strconv.FormatInt(int64(t), 10))
	case eval.Float:
		s, err := formatFloat(float64(t), w.style.floats)
		if err != nil {
			return fmt.Errorf("%s: %w", w.style.format, err)
		}
		w.buf.WriteString(s)
	case eval.String:
		w.writeString(string(t), depth)
	case *eval.Object:
		return w.object(t, depth)
	case *eval.Array:
		return w.array(t, depth)
	default:
		return fmt.Errorf("cannot render a value of type %s", v.TypeName())
	}
	return nil
}

func (w *bracketWriter) object(o *eval.Object, depth int) error {
	fields := make([]*eval.Field, 0, o.Len())
	for _, f := range o.Fields() {
		if !f.Hidden {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		w.buf.WriteString("{}")
		return nil
	}

	w.buf.WriteByte('{')
	for i, f := range fields {
		inner, err := f.Value.Value()
		if err != nil {
			return err
		}
		w.commentLines(f.Head, depth+1)
		w.nl(depth + 1)
		w.writeKey(f.Name)
		w.buf.WriteByte(':')
		if !w.style.compact {
			w.buf.WriteByte(' ')
		}
		if err := w.value(inner, depth+1); err != nil {
			return err
		}
		w.separator(i == len(fields)-1, false)
		w.lineComment(f.Line)
		w.commentLines(f.Foot, depth+1)
	}
	w.nl(depth)
	w.buf.WriteByte('}')
	return nil
}

func (w *bracketWriter) array(a *eval.Array, depth int) error {
	items := a.Items()
	if len(items) == 0 {
		w.buf.WriteString("[]")
		return nil
	}
	values := make([]eval.Value, len(items))
	for i, item := range items {
		v, err := item.Value.Value()
		if err != nil {
			return err
		}
		values[i] = v
	}

	// KYAML cuddles paired brackets, so "[{" opens a list of mappings and
	// "}, {" joins two of them. Only a list whose items are all collections
	// has brackets to pair up, and a comment written between two items has
	// to break the pair apart onto separate lines.
	cuddling := w.style.cuddle && allCollections(values)
	cuddled := make([]bool, len(items))
	for i, item := range items {
		if !cuddling || w.hasComments(item.Head) {
			continue
		}
		cuddled[i] = i == 0 || !w.trailedBy(items[i-1].Comments)
	}
	closeCuddled := cuddling && !w.trailedBy(items[len(items)-1].Comments)

	w.buf.WriteByte('[')
	// openDepth is where the last bracket written sits, which is the depth a
	// cuddled item opens and closes at.
	openDepth := depth
	for i, item := range items {
		itemDepth := depth + 1
		if cuddled[i] {
			itemDepth = openDepth
		} else {
			w.commentLines(item.Head, depth+1)
			w.nl(depth + 1)
		}
		if err := w.value(values[i], itemDepth); err != nil {
			return err
		}
		openDepth = itemDepth

		last := i == len(items)-1
		joined := closeCuddled
		if !last {
			joined = cuddled[i+1]
		}
		w.separator(last, joined)
		w.lineComment(item.Line)
		w.commentLines(item.Foot, depth+1)
	}
	if !closeCuddled {
		w.nl(depth)
	}
	w.buf.WriteByte(']')
	return nil
}

// separator writes the comma that follows an entry. The last entry of a
// collection gets one only where the format asks for trailing commas, and
// never when the closing bracket is about to cuddle onto it. joined says
// that whatever comes next shares this line.
func (w *bracketWriter) separator(last, joined bool) {
	if !last {
		w.buf.WriteByte(',')
		if joined {
			w.buf.WriteByte(' ')
		}
		return
	}
	if w.style.trailingComma && !joined {
		w.buf.WriteByte(',')
	}
}

// nl starts a new line indented to depth, or does nothing in a format that
// writes a document on one line.
func (w *bracketWriter) nl(depth int) {
	if w.style.compact {
		return
	}
	w.buf.WriteByte('\n')
	w.buf.WriteString(strings.Repeat(" ", depth*w.style.indent))
}

func (w *bracketWriter) hasComments(lines []string) bool {
	return w.style.comments != dropComments && len(lines) > 0
}

// trailedBy reports whether a comment follows an entry on its own line or at
// the end of it, which is what stops the next bracket from cuddling.
func (w *bracketWriter) trailedBy(c eval.Comments) bool {
	return w.style.comments != dropComments && (c.Line != "" || len(c.Foot) > 0)
}

// commentLines writes whole-line comments, each on a line of its own at
// depth. It serves both the comments above an entry and the ones below it.
func (w *bracketWriter) commentLines(lines []string, depth int) {
	if !w.hasComments(lines) {
		return
	}
	for _, c := range lines {
		w.nl(depth)
		w.buf.WriteString(w.commentText(c))
	}
}

func (w *bracketWriter) lineComment(c string) {
	if c == "" || w.style.comments == dropComments {
		return
	}
	w.buf.WriteByte(' ')
	w.buf.WriteString(w.commentText(c))
}

// commentText respells a yak comment, which always begins with "#", for the
// format being written.
func (w *bracketWriter) commentText(c string) string {
	if w.style.comments == slashComments {
		return "//" + strings.TrimPrefix(c, "#")
	}
	return c
}

func (w *bracketWriter) writeKey(name string) {
	if w.style.bareKeys && bareKey(name) {
		w.buf.WriteString(name)
		return
	}
	w.buf.WriteByte('"')
	escapeInto(&w.buf, name)
	w.buf.WriteByte('"')
}

func (w *bracketWriter) writeString(s string, depth int) {
	if w.style.foldStrings && strings.Contains(s, "\n") {
		w.foldedString(s, depth)
		return
	}
	w.buf.WriteByte('"')
	escapeInto(&w.buf, s)
	w.buf.WriteByte('"')
}

// foldedString writes a multi-line string the way KYAML does: one source line
// per output line, joined by YAML flow folds, with the newlines written as
// "\n" escapes. The result reads much like the block scalar it came from
// while staying a flow scalar.
func (w *bracketWriter) foldedString(s string, depth int) {
	lines := strings.Split(s, "\n")
	// Text that ends in a newline splits to an empty last line. Writing the
	// "\n" at the end of the line before it says the same thing in one line
	// less.
	trailingNewline := len(lines) > 1 && lines[len(lines)-1] == ""
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}
	pad := strings.Repeat(" ", (depth+1)*w.style.indent)

	w.buf.WriteString(`"\`)
	for i, line := range lines {
		esc := escapeFoldedLine(line)
		w.buf.WriteByte('\n')
		w.buf.WriteString(pad)
		// A line whose text is held by an escape already starts in the
		// right column; one that does not gets a space so that the whole
		// block lines up.
		if !strings.HasPrefix(esc, `\`) {
			w.buf.WriteByte(' ')
		}
		w.buf.WriteString(esc)
		if i < len(lines)-1 || trailingNewline {
			w.buf.WriteString(`\n`)
		}
		w.buf.WriteByte('\\')
	}
	w.buf.WriteByte('\n')
	w.buf.WriteString(pad)
	w.buf.WriteByte('"')
}

// escapeFoldedLine escapes one line of a folded string. A fold swallows the
// whitespace on either side of it, so a leading or trailing space has to be
// written as the "\ " escape to survive the round trip.
func escapeFoldedLine(line string) string {
	var b strings.Builder
	escapeInto(&b, line)
	out := b.String()
	if strings.HasSuffix(out, " ") {
		out = out[:len(out)-1] + `\ `
	}
	if strings.HasPrefix(out, " ") {
		out = `\` + out
	}
	return out
}

// ambiguousKeys are the words a YAML 1.1 parser reads as a boolean or as
// null, which therefore have to be quoted even where a bare key is allowed.
var ambiguousKeys = map[string]bool{
	"y": true, "n": true, "yes": true, "no": true, "on": true, "off": true,
	"true": true, "false": true, "null": true,
}

// bareKey reports whether a key can be written without quotes: letters,
// digits, "_" and "-", starting with a letter or "_", and not a word that
// would read as some other type.
func bareKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && (r >= '0' && r <= '9' || r == '-'):
		default:
			return false
		}
	}
	return !ambiguousKeys[strings.ToLower(s)]
}

func allCollections(values []eval.Value) bool {
	for _, v := range values {
		switch v.(type) {
		case *eval.Object, *eval.Array:
		default:
			return false
		}
	}
	return true
}
