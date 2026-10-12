// Package render writes evaluated yak values in one of the output formats.
package render

import (
	"fmt"
	"io"

	"github.com/daluz/yak/internal/eval"
)

// defaultIndent is the nesting width every format falls back to.
const defaultIndent = 2

// Options controls how documents are written.
type Options struct {
	Format Format
	// Indent is the number of spaces per nesting level.
	Indent int
	// KeepNullDocuments writes the documents that evaluated to null, which
	// are otherwise left out of the stream.
	KeepNullDocuments bool
}

// DefaultOptions returns the standard rendering settings.
func DefaultOptions() Options {
	return Options{Format: FormatYAML, Indent: defaultIndent}
}

// Documents writes every evaluated document to w in the requested format.
func Documents(w io.Writer, docs []eval.Value, opts Options) error {
	if opts.Indent <= 0 {
		opts.Indent = defaultIndent
	}
	if !opts.KeepNullDocuments {
		docs = withoutNull(docs)
	}

	// Resolve and encode everything before writing anything, so that a
	// failure in a later document does not leave a partial stream behind.
	for _, doc := range docs {
		if err := eval.Force(doc); err != nil {
			return err
		}
	}

	spec, ok := opts.Format.spec()
	if !ok {
		return fmt.Errorf("unsupported output format %s", opts.Format)
	}
	if spec.single {
		if err := singleDocument(opts.Format, docs); err != nil {
			return err
		}
	}
	out, err := spec.encode(docs, opts, spec.floats)
	if err != nil {
		return err
	}

	_, err = w.Write(out)
	return err
}

// withoutNull returns the documents that hold something. A document made of
// nothing but bindings is null, and so is an empty one, and neither is worth
// a "null" in the output.
func withoutNull(docs []eval.Value) []eval.Value {
	kept := make([]eval.Value, 0, len(docs))
	for _, doc := range docs {
		if _, null := doc.(eval.Null); null {
			continue
		}
		kept = append(kept, doc)
	}
	return kept
}

// entry is a field that is written out, or an item of a sequence, paired with
// the value it resolved to. An item has no name.
type entry struct {
	name string
	eval.Comments
	value eval.Value
}

// fieldEntries returns the fields of o that are written out, leaving out the
// hidden ones and those that hide a null. Documents forces every value before
// it hands a document to an encoder, so reading one here cannot fail.
func fieldEntries(o *eval.Object) []entry {
	out := make([]entry, 0, o.Len())
	for _, f := range o.Fields() {
		if omitted, _ := f.Omitted(); omitted {
			continue
		}
		v, _ := f.Value.Value()
		out = append(out, entry{name: f.Name, Comments: f.Comments, value: v})
	}
	return out
}

// itemEntries returns the items of a, which like fieldEntries relies on
// Documents having forced them.
func itemEntries(a *eval.Array) []entry {
	items := a.Items()
	out := make([]entry, len(items))
	for i, item := range items {
		v, _ := item.Value.Value()
		out[i] = entry{Comments: item.Comments, value: v}
	}
	return out
}

// unrenderable reports a value that no format can write, such as a function.
func unrenderable(v eval.Value) error {
	return fmt.Errorf("cannot render a value of type %s", v.TypeName())
}

// singleDocument reports the error a format that holds exactly one document
// gives when a template produced several, or nil when the count is fine.
func singleDocument(f Format, docs []eval.Value) error {
	if len(docs) == 1 {
		return nil
	}
	return fmt.Errorf("%s holds one document, but the template produced %d; use jsonl or yaml for a stream", f, len(docs))
}
