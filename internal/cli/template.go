package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/render"
)

func newTemplateCommand() *cobra.Command {
	var (
		contexts []string
		output   string
		format   string
	)

	cmd := &cobra.Command{
		Use:   "template FILE",
		Short: "Render a single template",
		Long: "Render one .yak template.\n\n" +
			"Context files may be YAML or JSON. They are merged in the order given,\n" +
			"with later files taking precedence, and are available to the template as\n" +
			"$context (or $$).\n\n" +
			"The output format defaults to YAML. Naming an output file with a known\n" +
			"extension selects the matching format, and --format overrides both.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := renderOptions(format, output)
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
			if output == "" || output == "-" {
				_, err = cmd.OutOrStdout().Write(buf.Bytes())
				return err
			}
			if err := os.WriteFile(output, buf.Bytes(), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&contexts, "context", "c", nil,
		"YAML or JSON file to merge into $context (repeatable; later files win)")
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"write the rendered output to this file instead of standard output")
	cmd.Flags().StringVarP(&format, "format", "f", "",
		"output format: "+render.FormatList()+" (default yaml, or the extension of --output)")

	return cmd
}

// renderOptions picks the output format: the flag if it was given, otherwise
// the extension of the output file, otherwise YAML.
func renderOptions(format, output string) (render.Options, error) {
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
		}
	}
	return opts, nil
}
