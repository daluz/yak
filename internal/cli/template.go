package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/render"
)

func newTemplateCommand() *cobra.Command {
	var (
		contexts  []string
		output    string
		automatic bool
		outputDir string
		format    string
		keepNull  bool
	)

	cmd := &cobra.Command{
		Use:   "template FILE",
		Short: "Render a single template",
		Long: "Render one .yak template.\n\n" +
			"Context files may be YAML or JSON. They are merged in the order given,\n" +
			"with later files taking precedence, and are available to the template as\n" +
			"$context (or $$).\n\n" +
			"The output format defaults to YAML. A template named for a format, such\n" +
			"as app.toml.yak, writes that format; naming an output file with a known\n" +
			"extension takes precedence over the template's name, and --format over\n" +
			"both.\n\n" +
			"--automatic-output names the output file after the template: app.toml.yak\n" +
			"yields app.toml, and app.yak yields app.yaml. --output-dir writes the\n" +
			"output file in another directory, creating it if it is missing.\n\n" +
			"A document that evaluates to null, such as one holding nothing but\n" +
			"bindings, is left out unless --keep-null-documents is given.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := renderOptions(format, args[0], output)
			if err != nil {
				return err
			}
			opts.KeepNullDocuments = keepNull
			dest, err := outputPath(args[0], output, automatic, outputDir, opts.Format)
			if err != nil {
				return err
			}
			// Buffer the output so a failure midway through evaluation never
			// leaves a half-written file behind.
			var buf bytes.Buffer
			err = engine.Template(engine.TemplateRequest{
				Path:         args[0],
				ContextPaths: contexts,
				Out:          &buf,
				Options:      opts,
			})
			if err != nil {
				return err
			}
			if dest == "" {
				_, err = cmd.OutOrStdout().Write(buf.Bytes())
				return err
			}
			if outputDir != "" {
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return fmt.Errorf("creating output directory: %w", err)
				}
			}
			if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&contexts, "context", "c", nil,
		"YAML or JSON file to merge into $context (repeatable; later files win)")
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
			return "", fmt.Errorf("--automatic-output needs a %s template, but got %q", engine.ExtTemplate, template)
		}
		// A template named for a format carries the extension that names
		// it, but the format actually written is what the file is named
		// for, so that -f json on app.toml.yak yields app.json.
		if _, ok := render.FormatForFile(stem); ok {
			stem = strings.TrimSuffix(stem, filepath.Ext(stem))
		}
		if dir != "" {
			// The directory says where the file goes, so only the name
			// the template gave it is kept.
			stem = filepath.Base(stem)
		}
		dest = stem + "." + f.String()
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

// templateStem returns a template's name without its .yak extension, or the
// empty string when it is not a template file at all, standard input
// included.
func templateStem(template string) string {
	if !strings.HasSuffix(template, engine.ExtTemplate) {
		return ""
	}
	return strings.TrimSuffix(template, engine.ExtTemplate)
}
