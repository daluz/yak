package render

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/eval"
)

// encodeTOML writes a single document as TOML.
//
// TOML has no document separator and no null, and its grammar puts every
// sub-table after the plain keys of the table it belongs to. The first two
// are reported as errors; the third means a document's keys come out grouped
// rather than in source order, which is the one place a yak renderer cannot
// keep the order it was written in.
func encodeTOML(docs []eval.Value, opts Options) ([]byte, error) {
	if err := singleDocument(FormatTOML, docs); err != nil {
		return nil, err
	}
	root, ok := docs[0].(*eval.Object)
	if !ok {
		return nil, fmt.Errorf("toml needs a mapping at the top level, but the document is a %s", docs[0].TypeName())
	}

	w := &tomlWriter{}
	if err := w.table(root, nil); err != nil {
		return nil, err
	}
	return w.buf.Bytes(), nil
}

type tomlWriter struct {
	buf bytes.Buffer
}

// tomlEntry is a visible field paired with the value it resolved to.
type tomlEntry struct {
	field *eval.Field
	value eval.Value
}

// table writes the body of the table at path: its plain keys first, then the
// sub-tables they contain.
func (w *tomlWriter) table(o *eval.Object, path []string) error {
	prefix := strings.Join(path, ".")
	keys, tables, err := partition(o)
	if err != nil {
		return err
	}

	for _, e := range keys {
		w.commentLines(e.field.Head)
		w.buf.WriteString(tomlKey(e.field.Name))
		w.buf.WriteString(" = ")
		if err := w.inline(e.value, join(prefix, e.field.Name)); err != nil {
			return err
		}
		w.lineComment(e.field.Line)
		w.buf.WriteByte('\n')
		w.commentLines(e.field.Foot)
	}

	for _, e := range tables {
		child := append(append([]string{}, path...), e.field.Name)
		switch t := e.value.(type) {
		case *eval.Object:
			// A table that holds nothing but other tables needs no header
			// of its own, since the header of any table inside it brings
			// it into being. One carrying a comment keeps its header,
			// because that is the only place the comment can go.
			if implied(t) && !commented(e.field.Comments) {
				if err := w.table(t, child); err != nil {
					return err
				}
				continue
			}
			w.blankLine()
			w.commentLines(e.field.Head)
			w.header("["+tomlPath(child)+"]", e.field.Line)
			if err := w.table(t, child); err != nil {
				return err
			}
			w.commentLines(e.field.Foot)
		case *eval.Array:
			// Every element opens a "[[table]]" of its own, so the
			// entry's own comments belong to the first of them.
			for i, item := range t.Items() {
				v, err := item.Value.Value()
				if err != nil {
					return err
				}
				w.blankLine()
				if i == 0 {
					w.commentLines(e.field.Head)
				}
				w.commentLines(item.Head)
				line := item.Line
				if i == 0 && line == "" {
					line = e.field.Line
				}
				w.header("[["+tomlPath(child)+"]]", line)
				elem, ok := v.(*eval.Object)
				if !ok {
					return fmt.Errorf("internal error: %s[%d] is a %s", join(prefix, e.field.Name), i, v.TypeName())
				}
				if err := w.table(elem, child); err != nil {
					return err
				}
				w.commentLines(item.Foot)
			}
			w.commentLines(e.field.Foot)
		}
	}
	return nil
}

func (w *tomlWriter) header(text, line string) {
	w.buf.WriteString(text)
	w.lineComment(line)
	w.buf.WriteByte('\n')
}

// inline writes a value that fits on the right of an "=", using an inline
// table or an inline array for any collection nested inside one.
func (w *tomlWriter) inline(v eval.Value, path string) error {
	switch t := v.(type) {
	case eval.Null:
		return fmt.Errorf("toml has no null, but %s is null", path)

	case eval.Bool:
		w.buf.WriteString(strconv.FormatBool(bool(t)))

	case eval.Int:
		w.buf.WriteString(strconv.FormatInt(int64(t), 10))

	case eval.Float:
		s, err := formatFloat(float64(t), tomlFloats)
		if err != nil {
			return fmt.Errorf("toml: %w", err)
		}
		w.buf.WriteString(s)

	case eval.String:
		w.buf.WriteString(tomlString(string(t)))

	case *eval.Object:
		w.buf.WriteByte('{')
		first := true
		for _, f := range t.Fields() {
			omitted, err := f.Omitted()
			if err != nil {
				return err
			}
			if omitted {
				continue
			}
			inner, err := f.Value.Value()
			if err != nil {
				return err
			}
			if !first {
				w.buf.WriteString(", ")
			}
			first = false
			w.buf.WriteString(tomlKey(f.Name))
			w.buf.WriteString(" = ")
			if err := w.inline(inner, join(path, f.Name)); err != nil {
				return err
			}
		}
		w.buf.WriteByte('}')

	case *eval.Array:
		w.buf.WriteByte('[')
		for i, item := range t.Items() {
			inner, err := item.Value.Value()
			if err != nil {
				return err
			}
			if i > 0 {
				w.buf.WriteString(", ")
			}
			if err := w.inline(inner, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		w.buf.WriteByte(']')

	default:
		return fmt.Errorf("cannot render a value of type %s", v.TypeName())
	}
	return nil
}

// commentLines writes whole-line comments, which TOML spells with "#" just
// as yak does.
func (w *tomlWriter) commentLines(lines []string) {
	for _, c := range lines {
		w.buf.WriteString(c)
		w.buf.WriteByte('\n')
	}
}

func (w *tomlWriter) lineComment(c string) {
	if c == "" {
		return
	}
	w.buf.WriteByte(' ')
	w.buf.WriteString(c)
}

// blankLine separates a table from what came before it, except at the very
// top of the output.
func (w *tomlWriter) blankLine() {
	b := w.buf.Bytes()
	if len(b) == 0 || bytes.HasSuffix(b, []byte("\n\n")) {
		return
	}
	w.buf.WriteByte('\n')
}

// partition splits a mapping's visible fields into the ones written as
// "key = value" and the ones written as a table of their own.
func partition(o *eval.Object) (keys, tables []tomlEntry, err error) {
	for _, f := range o.Fields() {
		omitted, err := f.Omitted()
		if err != nil {
			return nil, nil, err
		}
		if omitted {
			continue
		}
		v, err := f.Value.Value()
		if err != nil {
			return nil, nil, err
		}
		e := tomlEntry{field: f, value: v}
		if isTable(v) {
			tables = append(tables, e)
		} else {
			keys = append(keys, e)
		}
	}
	return keys, tables, nil
}

// implied reports whether a table would be written as a bare header with
// nothing under it.
func implied(o *eval.Object) bool {
	keys, tables, err := partition(o)
	return err == nil && len(keys) == 0 && len(tables) > 0
}

func commented(c eval.Comments) bool {
	return len(c.Head) > 0 || c.Line != "" || len(c.Foot) > 0
}

// isTable reports whether a value becomes a "[table]" header rather than a
// value on the right of an "=". An array qualifies when it holds nothing but
// mappings, which is what "[[table]]" arrays are made of.
func isTable(v eval.Value) bool {
	switch t := v.(type) {
	case *eval.Object:
		return true
	case *eval.Array:
		if t.Len() == 0 {
			return false
		}
		for _, item := range t.Items() {
			// Force has already run, so this cannot fail.
			inner, err := item.Value.Value()
			if err != nil {
				return false
			}
			if _, ok := inner.(*eval.Object); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// bareTOMLKey reports whether a key needs no quotes: letters, digits, "_"
// and "-", which is the whole of TOML's bare key syntax.
func bareTOMLKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func tomlKey(s string) string {
	if bareTOMLKey(s) {
		return s
	}
	return quote(s)
}

func tomlPath(parts []string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = tomlKey(p)
	}
	return strings.Join(out, ".")
}

// tomlString writes a string, reaching for a multi-line literal when the
// text has line breaks in it. Every quote inside one is escaped, so no run
// of three can close the string early.
func tomlString(s string) string {
	if !strings.Contains(s, "\n") {
		return quote(s)
	}
	var b strings.Builder
	b.WriteString("\"\"\"\n")
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		escapeInto(&b, line)
	}
	b.WriteString(`"""`)
	return b.String()
}

// join builds the dotted path used to point at a value in a diagnostic.
func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
