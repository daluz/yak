package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/yakfmt"
)

// typeNames are the values --type-names takes.
var typeNames = map[string]yakfmt.TypeNames{
	"keep":  yakfmt.KeepTypeNames,
	"short": yakfmt.ShortTypeNames,
	"long":  yakfmt.LongTypeNames,
}

func newFmtCommand() *cobra.Command {
	var write, list bool
	var typeNamesFlag string
	cmd := &cobra.Command{
		Use:   "fmt [PATH...]",
		Short: "Format templates and libraries",
		Long: "Format each .yak template and .yakl library, writing the result to\n" +
			"standard output. A directory formats every template and library beneath\n" +
			"it, skipping hidden files and directories and a package's vendor/.\n" +
			"Without a PATH, or given \"-\", fmt reads standard input.\n\n" +
			"Only whitespace changes, and line breaks stay where they were written,\n" +
			"unless --type-names respells the built-in types and their conversions.\n" +
			"A file that does not parse is reported, and the others are formatted\n" +
			"all the same.",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, ok := typeNames[typeNamesFlag]
			if !ok {
				return fmt.Errorf("--type-names takes %q, %q or %q, not %q", "keep", "short", "long", typeNamesFlag)
			}
			if len(args) == 0 {
				args = []string{"-"}
			}
			if write && slices.Contains(args, "-") {
				return errors.New("-w cannot rewrite standard input")
			}
			var errs []error
			for _, path := range args {
				files, err := modulesAt(path)
				if err != nil {
					return err
				}
				for _, file := range files {
					name, src, err := readInput(cmd, file)
					if err == nil {
						err = formatSource(cmd.OutOrStdout(), name, src, names, list, write)
					}
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false,
		"rewrite the files whose formatting differs rather than writing them to standard output")
	cmd.Flags().BoolVarP(&list, "list", "l", false,
		"list the files whose formatting differs rather than writing them to standard output")
	cmd.Flags().StringVar(&typeNamesFlag, "type-names", "keep",
		`spell the built-in types and their conversions "short" (int), "long" (integer), or "keep" them as written`)
	return cmd
}

// formatSource formats src and writes it to out, unless it is to be listed
// or written back to the file name.
func formatSource(out io.Writer, name string, src []byte, names yakfmt.TypeNames, list, write bool) error {
	formatted, err := yakfmt.Source(name, src, names)
	if err != nil {
		return err
	}
	if !list && !write {
		_, err := out.Write(formatted)
		return err
	}
	if bytes.Equal(src, formatted) {
		return nil
	}
	if list {
		if _, err := fmt.Fprintln(out, name); err != nil {
			return err
		}
	}
	if write {
		info, err := os.Stat(name)
		if err != nil {
			return err
		}
		return os.WriteFile(name, formatted, info.Mode().Perm())
	}
	return nil
}
