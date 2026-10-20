package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/schemaimport"
)

func newSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Work with the schemas templates and libraries declare",
	}
	cmd.AddCommand(newSchemaCheckCommand(), newSchemaImportCommand())
	return cmd
}

func newSchemaImportCommand() *cobra.Command {
	var output, format string
	cmd := &cobra.Command{
		Use:   "import FILE...",
		Short: "Translate CRDs, JSON Schemas or OpenAPI documents into a library of schemas",
		Long: "Translate the schemas each FILE declares into one .yakl library. \"-\"\n" +
			"reads standard input.\n\n" +
			"crd: every served version of a Kubernetes CustomResourceDefinition, or of\n" +
			"each one a List holds, as kubectl writes, becomes a schema named for its\n" +
			"kind and version, such as WidgetV1. It fills in the apiVersion and kind\n" +
			"of the object, and takes any metadata.\n\n" +
			"jsonschema: a JSON Schema document, draft-04 to 2020-12, becomes a schema\n" +
			"named for its title, or else its file, and each of its $defs and\n" +
			"definitions a schema named for its key.\n\n" +
			"openapi: each schema under the components.schemas of an OpenAPI v3\n" +
			"document becomes a schema named for its key.\n\n" +
			"Types, enums, defaults, required fields, the limits on numbers, strings,\n" +
			"sequences and mappings, and the $refs, allOf, anyOf and oneOf of JSON\n" +
			"Schema and OpenAPI are imported. Every keyword that is not, such as\n" +
			"x-kubernetes-validations or format, is left out with a warning.\n\n" +
			"--format names what the files hold. Without it, a file holding a\n" +
			"CustomResourceDefinition is read as crd, one with an \"openapi: 3.x\" as\n" +
			"openapi, one whose $schema is a json-schema.org one as jsonschema, and\n" +
			"any other is an error.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files := make([]schemaimport.File, len(args))
			for i, path := range args {
				var err error
				if path == "-" {
					files[i].Name = "<stdin>"
					files[i].Data, err = io.ReadAll(cmd.InOrStdin())
				} else {
					files[i].Name = path
					files[i].Data, err = os.ReadFile(path)
				}
				if err != nil {
					return err
				}
			}
			if format == "" {
				var err error
				if format, err = detectFormat(files); err != nil {
					return err
				}
			}
			lib, warnings, err := schemaimport.Import(format, files)
			for _, w := range warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "yak: warning: %s\n", w)
			}
			if err != nil {
				return err
			}
			if output == "" || output == "-" {
				_, err = cmd.OutOrStdout().Write(lib)
				return err
			}
			return writeOutput(output, lib, false)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"write the library to this file instead of standard output")
	cmd.Flags().StringVar(&format, "format", "",
		"input format: "+strings.Join(schemaimport.Formats, ", ")+" (default: detected from each file)")
	return cmd
}

// detectFormat answers the one format every file is detected to hold.
func detectFormat(files []schemaimport.File) (string, error) {
	var format, first string
	for _, f := range files {
		got, err := schemaimport.Detect(f)
		if err != nil {
			return "", err
		}
		switch {
		case got == "":
			return "", fmt.Errorf("%s: cannot tell its format; give --format (%s)",
				f.Name, strings.Join(schemaimport.Formats, ", "))
		case format == "":
			format, first = got, f.Name
		case got != format:
			return "", fmt.Errorf("%s holds %s, but %s holds %s; import them separately", first, format, f.Name, got)
		}
	}
	return format, nil
}

func newSchemaCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check PATH...",
		Short: "Check the schemas of templates and libraries",
		Long: "Check the schemas each .yak template and .yakl library declares, and the\n" +
			"tags it writes, without rendering it or reading a context: every schema\n" +
			"a type or a tag names has to be in scope, a module's included, and every\n" +
			"\"<<\" in a mapping type has to merge a mapping type without coming back\n" +
			"to itself. A directory checks every template and library beneath it,\n" +
			"skipping hidden files and directories and a package's vendor/.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, path := range args {
				files := []string{path}
				if info, err := os.Stat(path); err == nil && info.IsDir() {
					if files, err = sourcesIn(path, engine.ExtTemplate, module.ExtLibrary); err != nil {
						return err
					}
					if len(files) == 0 {
						return fmt.Errorf("no %s templates or %s libraries beneath %s",
							engine.ExtTemplate, module.ExtLibrary, path)
					}
				}
				for _, file := range files {
					if err := engine.CheckSchemas(file); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}
