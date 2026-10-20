// Package engine wires the parser, evaluator and renderer into the pipeline
// the commands drive.
package engine

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/render"
	_ "github.com/daluz/yak/internal/stdlib"
)

// ExtTemplate is the extension of a template, which is the one a module that
// can be rendered is written with.
const ExtTemplate = module.Ext

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
	// NonStrict turns strict mode off; the zero value renders strictly.
	NonStrict bool
}

// Template renders one template file to the request's writer.
func Template(req TemplateRequest) error {
	if strings.HasSuffix(req.Path, module.ExtLibrary) {
		return fmt.Errorf("%s is a library, which can be imported but not rendered", req.Path)
	}
	name, src, err := readSource(req.Path)
	if err != nil {
		return err
	}
	context, err := ctxfile.Load(req.ContextPaths)
	if err != nil {
		return err
	}
	workdir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("finding the working directory: %w", err)
	}
	run := eval.Run{FilePath: name, ContextPaths: req.ContextPaths, Workdir: workdir, NonStrict: req.NonStrict}
	return Render(run, src, context, req.Out, req.Options)
}

// Render parses, evaluates and writes a source buffer. It is the seam used by
// tests and by future commands that already hold their input in memory. The
// run names the source and is what "$yak", "$filepath", "$contextpaths" and
// "$workdir" report, along with the format opts selects.
func Render(run eval.Run, src []byte, context eval.Value, w io.Writer, opts render.Options) error {
	run.Format = opts.Format.String()
	stream, err := parser.Parse(run.FilePath, src)
	if err != nil {
		return err
	}
	docs := make([]eval.Value, 0, len(stream.Docs))
	for _, doc := range stream.Docs {
		v, err := eval.Document(doc, context, run, module.NewLoader(run.Strict()))
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
	src, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("reading template: %w", err)
	}
	return path, src, nil
}
