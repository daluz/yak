package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/deps"
	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/render"
)

func newRenderCommand() *cobra.Command {
	var (
		contexts   []string
		output     string
		automatic  bool
		outputDir  string
		format     string
		keepNull   bool
		strict     bool
		skipChecks bool
		skipCtx    bool
		plainHTTP  bool
	)

	cmd := &cobra.Command{
		Use:   "render PATH",
		Short: "Render a template, a directory or a remote package",
		Long: "Render one .yak template, or every .yak template beneath a directory or\n" +
			"in a remote package, given as git+https://... or oci://...\n" +
			"A .yakl library can be imported but not rendered, so rendering a\n" +
			"directory skips libraries, and rendering one directly is an error.\n\n" +
			"Context files may be YAML or JSON. They are merged in the order given,\n" +
			"with later files taking precedence, and are available to the template as\n" +
			"$context, $ctx or $$. The files a package lists as package.context in its\n" +
			"yak.pkg merge first, unless --skip-default-context is given.\n\n" +
			"The output format defaults to YAML. A template named for a format, such\n" +
			"as app.toml.yak, writes that format; naming an output file with a known\n" +
			"extension takes precedence over the template's name, and --format over\n" +
			"both.\n\n" +
			"--automatic-output names the output file after the template: app.toml.yak\n" +
			"yields app.toml, and app.yak yields app.yaml. --output-dir writes the\n" +
			"output file in another directory, creating it if it is missing.\n\n" +
			"A directory writes one file per template, so it needs --output-dir or\n" +
			"--automatic-output: the files go beside their templates, or into the same\n" +
			"tree beneath --output-dir. Hidden files and directories are skipped, and\n" +
			"nothing is written unless every template renders.\n\n" +
			"A document that evaluates to null, such as one holding nothing but\n" +
			"bindings, is left out unless --keep-null-documents is given.\n\n" +
			"Rendering is strict unless --strict=false is given: the same template and\n" +
			"context always render the same output, so modules such as std.random and\n" +
			"std.env are refused, and std.files reads only beneath the directory of the\n" +
			"file that calls it.\n\n" +
			"--skip-schema-checks lets every value through the schemas, which then\n" +
			"only fill in defaults.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := renderRequest{contexts: contexts, format: format, keepNull: keepNull, nonStrict: !strict,
				skipChecks: skipChecks, skipCtx: skipCtx}
			if module.IsRemote(args[0]) {
				fetch := deps.Options{Log: cmd.ErrOrStderr(), PlainHTTP: plainHTTP}
				return renderRemote(args[0], req, output, automatic, outputDir, fetch)
			}
			if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
				return renderDir(args[0], req, output, automatic, outputDir)
			}
			opts, err := req.options(args[0], output)
			if err != nil {
				return err
			}
			dest, err := outputPath(args[0], output, automatic, outputDir, opts.Format)
			if err != nil {
				return err
			}
			out, err := req.render(args[0], opts)
			if err != nil {
				return err
			}
			if dest == "" {
				_, err = cmd.OutOrStdout().Write(out)
				return err
			}
			return writeOutput(dest, out, outputDir != "")
		},
	}

	cmd.Flags().StringArrayVarP(&contexts, "context", "c", nil,
		"YAML or JSON file to merge into $context (repeatable; later files win)")
	cmd.Flags().BoolVar(&skipCtx, "skip-default-context", false,
		"leave out the default context files the template's package lists")
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"write the rendered output to this file instead of standard output")
	cmd.Flags().BoolVarP(&automatic, "automatic-output", "O", false,
		"write the rendered output to a file named after the template and its format")
	cmd.Flags().StringVar(&outputDir, "output-dir", "",
		"directory to write the output file in, with --output or --automatic-output")
	cmd.Flags().StringVarP(&format, "format", "f", "",
		"output format: "+render.FormatList()+" (default yaml, or the extension of --output or of the template)")
	cmd.Flags().BoolVar(&keepNull, "keep-null-documents", false,
		"write the documents that evaluated to null instead of leaving them out")
	plainHTTPFlag(cmd, &plainHTTP)
	cmd.Flags().BoolVar(&strict, "strict", true,
		"render deterministically; --strict=false admits std.random, std.env and reading files anywhere")
	cmd.Flags().BoolVar(&skipChecks, "skip-schema-checks", false,
		"fill in the defaults schemas declare without checking any value against them")
	cmd.MarkFlagsMutuallyExclusive("output", "automatic-output")

	return cmd
}

// renderOptions picks the output format: the flag if it was given, otherwise
// the extension of the output file, otherwise the one the template is named
// for, otherwise YAML.
func renderOptions(format, template, output string) (render.Options, error) {
	opts := render.DefaultOptions()
	if format != "" {
		f, err := render.ParseFormat(format)
		if err != nil {
			return opts, err
		}
		opts.Format = f
		return opts, nil
	}
	if output != "" && output != "-" {
		if f, ok := render.FormatForFile(output); ok {
			opts.Format = f
			return opts, nil
		}
	}
	if f, ok := render.FormatForFile(templateStem(template)); ok {
		opts.Format = f
	}
	return opts, nil
}

// outputPath resolves where the rendered output goes, returning the empty
// string for standard output.
func outputPath(template, output string, automatic bool, dir string, f render.Format) (string, error) {
	dest := output
	if dest == "-" {
		dest = ""
	}
	if automatic {
		stem := templateStem(template)
		if stem == "" {
			return "", fmt.Errorf("--automatic-output needs a %s template, but got %q", module.Ext, template)
		}
		if dir != "" {
			// The directory says where the file goes, so only the name
			// the template gave it is kept.
			stem = filepath.Base(stem)
		}
		dest = outputName(stem, f)
	}
	if dir == "" {
		return dest, nil
	}
	if dest == "" {
		return "", errors.New("--output-dir needs --output or --automatic-output")
	}
	if filepath.IsAbs(dest) {
		return "", fmt.Errorf("--output-dir cannot hold the absolute path %q", dest)
	}
	return filepath.Join(dir, dest), nil
}

// outputName names the file that the template whose name without .yak is
// stem writes in format f.
func outputName(stem string, f render.Format) string {
	// A template named for a format carries the extension that names it,
	// but the format actually written is what the file is named for, so
	// that -f json on app.toml.yak yields app.json.
	if _, ok := render.FormatForFile(stem); ok {
		stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	}
	return stem + "." + f.String()
}

// renderRequest holds what every template one command renders has in
// common.
type renderRequest struct {
	contexts   []string
	format     string
	keepNull   bool
	nonStrict  bool
	skipChecks bool
	skipCtx    bool
}

func (r renderRequest) options(template, output string) (render.Options, error) {
	opts, err := renderOptions(r.format, template, output)
	opts.KeepNullDocuments = r.keepNull
	return opts, err
}

// render renders a template into memory, so that a failure midway through
// evaluation never leaves a half-written file behind.
func (r renderRequest) render(template string, opts render.Options) ([]byte, error) {
	var buf bytes.Buffer
	err := engine.Template(engine.TemplateRequest{
		Path:               template,
		ContextPaths:       r.contexts,
		SkipDefaultContext: r.skipCtx,
		Out:                &buf,
		Options:            opts,
		NonStrict:          r.nonStrict,
		SkipChecks:         r.skipChecks,
	})
	return buf.Bytes(), err
}

// renderDir renders every template beneath dir, writing each beside its
// template, or at the same place beneath outputDir when it is given. Nothing
// is written until every template has rendered.
func renderDir(dir string, req renderRequest, output string, automatic bool, outputDir string) error {
	if output != "" {
		return fmt.Errorf("--output names one file, but the directory %s writes one per template; use --output-dir or --automatic-output", dir)
	}
	if !automatic && outputDir == "" {
		return fmt.Errorf("the directory %s writes one file per template; give --output-dir or --automatic-output", dir)
	}
	templates, err := sourcesIn(dir, module.Ext)
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		return fmt.Errorf("no %s templates beneath %s", module.Ext, dir)
	}
	base := dir
	if outputDir != "" {
		base = outputDir
	}
	type rendered struct {
		dest string
		out  []byte
	}
	var all []rendered
	writers := map[string]string{}
	for _, template := range templates {
		opts, err := req.options(template, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, template)
		if err != nil {
			return err
		}
		dest := filepath.Join(base, outputName(templateStem(rel), opts.Format))
		if other, ok := writers[dest]; ok {
			return fmt.Errorf("%s and %s would both write %s", other, template, dest)
		}
		writers[dest] = template
		out, err := req.render(template, opts)
		if err != nil {
			return err
		}
		all = append(all, rendered{dest, out})
	}
	for _, r := range all {
		if err := writeOutput(r.dest, r.out, true); err != nil {
			return err
		}
	}
	return nil
}

// renderRemote fetches the remote package ref names, with what it depends
// on, and renders it into outputDir.
func renderRemote(ref string, req renderRequest, output string, automatic bool, outputDir string, fetch deps.Options) error {
	if output != "" || automatic || outputDir == "" {
		return fmt.Errorf("the remote package %s writes one file per template; give --output-dir, without --output or --automatic-output", ref)
	}
	src, err := module.ParseSource(ref)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "yak-render-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dir, err := deps.Fetch(src, tmp, fetch)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("%s names no directory of its repository", ref)
	}
	return renderDir(dir, req, "", false, outputDir)
}

// sourcesIn lists the files beneath dir that end in one of exts, leaving out
// whatever is hidden and the vendor directory of a package.
func sourcesIn(dir string, exts ...string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && d.Name() == module.VendorDir {
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), module.PkgFile)); err == nil {
				return filepath.SkipDir
			}
		}
		if !d.IsDir() && slices.ContainsFunc(exts, func(ext string) bool { return strings.HasSuffix(path, ext) }) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// modulesAt answers path, or the templates and libraries beneath it when it is
// a directory.
func modulesAt(path string) ([]string, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return []string{path}, nil
	}
	return sourcesIn(path, module.Ext, module.ExtLibrary)
}

// readInput reads the file at path, or standard input for "-", and answers
// the name to report it by.
func readInput(cmd *cobra.Command, path string) (string, []byte, error) {
	if path == "-" {
		src, err := io.ReadAll(cmd.InOrStdin())
		return "<stdin>", src, err
	}
	src, err := os.ReadFile(path)
	return path, src, err
}

// writeOutput writes out to dest, creating its directory first when mkdir is
// set.
func writeOutput(dest string, out []byte, mkdir bool) error {
	if mkdir {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}
	if err := os.WriteFile(dest, out, 0o644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

// templateStem returns a template's name without its .yak extension, or the
// empty string when it is not a template file at all, standard input
// included.
func templateStem(template string) string {
	if !strings.HasSuffix(template, module.Ext) {
		return ""
	}
	return strings.TrimSuffix(template, module.Ext)
}
