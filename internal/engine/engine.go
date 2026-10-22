// Package engine wires the parser, evaluator and renderer into the pipeline
// the commands drive.
package engine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/render"
	_ "github.com/daluz/yak/internal/stdlib"
)

// TemplateRequest describes a single template rendering.
type TemplateRequest struct {
	// Path is the template to render, or "-" to read standard input.
	Path string
	// ContextPaths are merged in order, with later files taking precedence,
	// into the context the template's package context file reads.
	ContextPaths []string
	// Context, when set, is the context itself, and ContextPaths is not
	// read.
	Context eval.Value
	// SkipPackageContext hands the template the merged context files
	// without evaluating its package's context file.
	SkipPackageContext bool
	Out                io.Writer
	// Options selects the output format and its indentation. The zero
	// value renders YAML at the default indent.
	Options render.Options
	// NonStrict turns strict mode off; the zero value renders strictly.
	NonStrict bool
	// SkipChecks lets every value through a schema, which then only fills
	// in defaults.
	SkipChecks bool
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
	context := req.Context
	if context == nil {
		if context, err = ctxfile.Load(req.ContextPaths); err != nil {
			return err
		}
	}
	workdir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("finding the working directory: %w", err)
	}
	run := eval.Run{FilePath: name, Workdir: workdir, NonStrict: req.NonStrict, SkipChecks: req.SkipChecks}
	if !req.SkipPackageContext {
		pkg, err := module.FindPackage(filepath.Dir(name))
		if err != nil {
			return err
		}
		if pkg != nil && pkg.Context != "" && !pkg.IsContext(name) {
			if context, err = packageContext(pkg.ContextPath(), context, run); err != nil {
				return err
			}
		}
	}
	return Render(run, src, context, req.Out, req.Options)
}

// packageContext evaluates the context file at path against context and
// answers what it renders, read back as a context file is, so that a
// template sees plain data whatever the context file computed.
func packageContext(path string, context eval.Value, run eval.Run) (eval.Value, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the package context: %w", err)
	}
	run.FilePath = path
	var buf bytes.Buffer
	if err := Render(run, src, context, &buf, render.DefaultOptions()); err != nil {
		return nil, err
	}
	return ctxfile.Decode(path, buf.Bytes())
}

// CheckSchemas checks the schemas a template or a library declares and the
// tags it writes, without rendering it or reading a context.
func CheckSchemas(path string) error {
	name, src, err := readSource(path)
	if err != nil {
		return err
	}
	stream, err := parser.Parse(name, src)
	if err != nil {
		return err
	}
	run := eval.Run{FilePath: name}
	for _, doc := range stream.Docs {
		if err := eval.CheckSchemas(doc, run, module.NewLoader(run.Strict())); err != nil {
			return err
		}
	}
	return nil
}

// Render parses, evaluates and writes a source buffer. The run names the
// source and is what "$yak", "$filepath" and "$workdir"
// report, along with the format opts selects.
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
