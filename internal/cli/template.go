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
	)

	cmd := &cobra.Command{
		Use:   "template FILE",
		Short: "Render a single template",
		Long: "Render one .yak template to YAML.\n\n" +
			"Context files may be YAML or JSON. They are merged in the order given,\n" +
			"with later files taking precedence, and are available to the template as\n" +
			"$context (or $$).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Buffer the output so a failure midway through evaluation never
			// leaves a half-written file behind.
			var buf bytes.Buffer
			err := engine.Template(engine.TemplateRequest{
				Path:         args[0],
				ContextPaths: contexts,
				Out:          &buf,
				Options:      render.DefaultOptions(),
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
		"write rendered YAML to this file instead of standard output")

	return cmd
}
