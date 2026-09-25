// Package engine wires the parser, evaluator and renderer into the pipeline
// the commands drive.
package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/render"
)

// Extensions recognised by the tool.
const (
	ExtTemplate = ".yak"
	ExtLibrary  = ".libyak"
)

// TemplateRequest describes a single template rendering.
type TemplateRequest struct {
	// Path is the template to render, or "-" to read standard input.
	Path string
	// ContextPaths are merged in order, with later files taking precedence.
	ContextPaths []string
	Out          io.Writer
	// Options selects the output format and its indentation. The zero
	// value renders YAML at the default indent.
	Options render.Options
}

// Template renders one template file to the request's writer.
func Template(req TemplateRequest) error {
	name, src, err := readSource(req.Path)
	if err != nil {
		return err
	}
	context, err := ctxfile.Load(req.ContextPaths)
	if err != nil {
		return err
	}
	return Render(name, src, context, req.Out, req.Options)
}

// Render parses, evaluates and writes a source buffer. It is the seam used by
// tests and by future commands that already hold their input in memory.
func Render(name string, src []byte, context eval.Value, w io.Writer, opts render.Options) error {
	stream, err := parser.Parse(name, src)
	if err != nil {
		return err
	}
	docs := make([]eval.Value, 0, len(stream.Docs))
	for _, doc := range stream.Docs {
		v, err := eval.Document(doc, context)
		if err != nil {
			return err
		}
		docs = append(docs, v)
	}
	return render.Documents(w, docs, opts)
}

func readSource(path string) (string, []byte, error) {
	if path == "-" {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", nil, fmt.Errorf("reading standard input: %w", err)
		}
		return "<stdin>", src, nil
	}
	if filepath.Ext(path) == ExtLibrary {
		return "", nil, fmt.Errorf("%s is a library file; %s files are meant to be imported, not rendered", path, ExtLibrary)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("reading template: %w", err)
	}
	return path, src, nil
}
