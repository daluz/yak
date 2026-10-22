package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/deps"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/yakspec"
)

func newSpecCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Work with spec files, which say what a release renders",
		Long: "A .yakspec spec file names the packages to render, the context and options\n" +
			"they are rendered with, and where each of their outputs goes.",
	}
	cmd.AddCommand(newSpecInitCommand(), newSpecRenderCommand())
	return cmd
}

func newSpecRenderCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "render FILE.yakspec",
		Short: "Render the packages a spec file names",
		Long: "Render every package the spec file names, local or remote, against its\n" +
			"context, and write each output where the spec file's rules send it. The\n" +
			"spec file sets every option itself, so there are no flags.\n\n" +
			"The spec file is evaluated outside strict mode against an empty context;\n" +
			"the packages render in strict mode unless it sets strict: false. When two\n" +
			"packages would write the same file, the later one in packages wins, and\n" +
			"nothing is written unless every template renders.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderSpec(args[0], cmd.ErrOrStderr())
		},
	}
}

func newSpecInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init [FILE.yakspec]",
		Short: "Write a spec file to start from",
		Long: "Write FILE.yakspec, or " + yakspec.DefaultFile + " in the current directory, as an example\n" +
			"spec file to edit. It refuses a file that exists, and a directory inside a\n" +
			"package, since a spec file belongs to none.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := yakspec.DefaultFile
			if len(args) == 1 {
				path = args[0]
			}
			return yakspec.Init(path)
		},
	}
}

// renderSpec renders the packages the spec file at path names, each
// against its context and to where its rules send it.
func renderSpec(path string, log io.Writer) error {
	spec, err := yakspec.Load(path)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "yak-render-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	req := renderRequest{format: spec.Format, keepNull: spec.KeepNullDocuments, nonStrict: spec.NonStrict,
		skipChecks: spec.SkipSchemaChecks, skipCtx: spec.SkipPackageContext}
	fetch := deps.Options{Log: log, PlainHTTP: spec.PlainHTTP}
	var all []rendered
	for i, p := range spec.Packages {
		dir := p.Source
		if module.IsRemote(dir) {
			if dir, err = fetchRemote(dir, filepath.Join(tmp, strconv.Itoa(i)), fetch); err != nil {
				return err
			}
		}
		pkg, err := module.ReadPackage(dir)
		if err == nil && pkg == nil {
			err = fmt.Errorf("%s is not a package: it has no %s", dir, module.PkgFile)
		}
		if err != nil {
			return err
		}
		templates, err := templatesIn(dir)
		if err != nil {
			return err
		}
		req.context = p.Context
		writers := map[string]string{}
		for _, template := range templates {
			rel, err := filepath.Rel(dir, template)
			if err != nil {
				return err
			}
			target := p.Target(pkg.Name, filepath.ToSlash(rel))
			if target.Drop {
				continue
			}
			output := ""
			if target.File {
				output = target.Path
			}
			opts, err := req.options(template, output)
			if err != nil {
				return err
			}
			dest := filepath.Join(spec.Output, filepath.FromSlash(target.Path))
			if !target.File {
				dest = filepath.Join(dest, outputName(templateStem(filepath.FromSlash(target.Rest)), opts.Format))
			}
			if other, ok := writers[dest]; ok {
				return fmt.Errorf("%s and %s would both write %s", other, template, dest)
			}
			writers[dest] = template
			out, err := req.render(template, opts)
			if err != nil {
				return err
			}
			// Written in order, a later package's file replaces an earlier one's.
			all = append(all, rendered{dest, out})
		}
	}
	if err := spec.CheckRules(); err != nil {
		return err
	}
	return writeAll(all)
}
