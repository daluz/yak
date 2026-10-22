// Package cli defines the yak command line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/daluz/yak/internal/version"
	"github.com/spf13/cobra"
)

// NewRootCommand builds the yak command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "yak",
		Short: "Templatize structured configuration files with a small, explicit language",
		Long: "yak renders YAML, JSON, TOML and other configuration files from\n" +
			"templates written in a restricted, unambiguous dialect of YAML with\n" +
			"string interpolation and relative references.",
		Version:       version.Current,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newRenderCommand(),
		newFmtCommand(),
		newPackageCommand(),
		newSchemaCommand(),
		newSpecCommand(),
		shortcut(newAddCommand()),
		shortcut(newRemoveCommand()),
		shortcut(newSyncCommand()),
	)
	return root
}

// Main runs the CLI and returns the process exit code.
func Main() int {
	err := NewRootCommand().Execute()
	if err == nil {
		return 0
	}
	errs := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	}
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "yak: %v\n", err)
	}
	return 1
}
