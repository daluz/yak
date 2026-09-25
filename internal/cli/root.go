// Package cli defines the yak command line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is overridden at build time with -ldflags "-X ...cli.version=...".
var version = "dev"

// NewRootCommand builds the yak command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "yak",
		Short: "Templatize YAML with a small, explicit language",
		Long: "yak renders YAML templates written in a restricted, unambiguous\n" +
			"dialect of YAML with string interpolation and relative references.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newTemplateCommand())
	return root
}

// Main runs the CLI and returns the process exit code.
func Main() int {
	if err := NewRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "yak: %v\n", err)
		return 1
	}
	return 0
}
