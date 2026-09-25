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

	// Resolve and encode everything before writing anything, so that a
	// failure in a later document does not leave a partial stream behind.
	for _, doc := range docs {
		if err := eval.Force(doc); err != nil {
			return err
		}
	}

	var (
		out []byte
		err error
	)
	switch opts.Format {
	case FormatYAML:
		out, err = encodeYAML(docs, opts)
	case FormatKYAML, FormatJSON, FormatJSONC, FormatJWCC, FormatJSONL:
		out, err = encodeBracketed(docs, opts)
	case FormatTOML:
		out, err = encodeTOML(docs, opts)
	default:
		err = fmt.Errorf("unsupported output format %s", opts.Format)
	}
	if err != nil {
		return err
	}

	_, err = w.Write(out)
	return err
}

// singleDocument reports the error a format that holds exactly one document
// gives when a template produced several, or nil when the count is fine.
func singleDocument(f Format, docs []eval.Value) error {
	if len(docs) == 1 {
		return nil
	}
	return fmt.Errorf("%s holds one document, but the template produced %d; use jsonl or yaml for a stream", f, len(docs))
}
