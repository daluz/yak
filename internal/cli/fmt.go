package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/yakfmt"
)

func newFmtCommand() *cobra.Command {
	var write, list bool
	cmd := &cobra.Command{
		Use:   "fmt [PATH...]",
		Short: "Format templates and libraries",
		Long: "Format each .yak template and .yakl library, writing the result to\n" +
			"standard output. A directory formats every template and library beneath\n" +
			"it, skipping hidden files and directories and a package's vendor/.\n" +
			"Without a PATH, or given \"-\", fmt reads standard input.\n\n" +
			"Only whitespace changes, and line breaks stay where they were written.\n" +
			"Each level of indentation becomes two spaces; a comma, a colon and the\n" +
			"\"-\" of a sequence item are followed by one space, and binary operators,\n" +
			"\"=\" and \"=>\" are surrounded by one; brackets and parentheses hold no\n" +
			"space inside; runs of blank lines become one; and the trailing comments\n" +
			"of consecutive lines are aligned. Strings and comments are kept as they\n" +
			"are. A file that does not parse is reported, and the others are\n" +
			"formatted all the same.\n\n" +
			"-l lists the files whose formatting differs instead, and -w rewrites\n" +
			"them in place.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"-"}
			}
			out := cmd.OutOrStdout()
			var errs []error
			for _, path := range args {
				if path == "-" {
					if write {
						return errors.New("-w cannot rewrite standard input")
					}
					src, err := io.ReadAll(cmd.InOrStdin())
					if err != nil {
						return fmt.Errorf("reading standard input: %w", err)
					}
					if err := formatSource(out, "<stdin>", "", src, list, false); err != nil {
						return err
					}
					continue
				}
				files := []string{path}
				if info, err := os.Stat(path); err == nil && info.IsDir() {
					if files, err = sourcesIn(path, engine.ExtTemplate, module.ExtLibrary); err != nil {
						return err
					}
				}
				for _, file := range files {
					src, err := os.ReadFile(file)
					if err == nil {
						err = formatSource(out, file, file, src, list, write)
					}
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false,
		"rewrite each file whose formatting differs instead of writing it to standard output")
	cmd.Flags().BoolVarP(&list, "list", "l", false,
		"list the files whose formatting differs instead of writing them to standard output")
	return cmd
}

// formatSource formats the source of the file name, read from path, and
// writes it to out unless it is to be listed or written back to path.
func formatSource(out io.Writer, name, path string, src []byte, list, write bool) error {
	formatted, err := yakfmt.Source(name, src)
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
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, formatted, info.Mode().Perm())
	}
	return nil
}
