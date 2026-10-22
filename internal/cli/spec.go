package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/deps"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/render"
	"github.com/daluz/yak/internal/yakspec"
)

func newSpecCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Work with spec files, which say what a release renders",
		Long: "A .yakspec spec file names the packages to render, the context and options\n" +
			"they are rendered with, and where each of their outputs goes.",
	}
	cmd.AddCommand(newSpecRenderCommand(), newSpecInitCommand())
	return cmd
}

func newSpecRenderCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "render FILE.yakspec",
		Short: "Render the packages a spec file names",
		Long: "Render every package the spec file names, local or remote, against its\n" +
			"context, and write each output where the spec file's rules send it. The\n" +
			"spec file sets every option itself, so there are no flags.\n\n" +
			"The spec file is evaluated out of strict mode against an empty context;\n" +
			"the packages render in strict mode unless it sets strict: false. When two\n" +
			"packages would write the same file, the later one in packages wins, and\n" +
			"nothing is written unless every template renders.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !strings.HasSuffix(args[0], yakspec.Ext) {
				return fmt.Errorf("%s is not a %s spec file", args[0], yakspec.Ext)
			}
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
// against its context and to where its rules send it. Two templates of one
// package writing the same file are an error; between packages, the later
// one in the list wins. Nothing is written unless every template renders.
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
	written := map[string]int{}
	for i, p := range spec.Packages {
		dir := p.Source
		if module.IsRemote(dir) {
			if dir, err = fetchRemote(dir, filepath.Join(tmp, strconv.Itoa(i)), fetch); err != nil {
				return err
			}
		}
		pkg, err := packageAt(dir)
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
			var opts render.Options
			var dest string
			if target.File {
				opts, err = req.options(template, target.Path)
				dest = filepath.Join(spec.Output, filepath.FromSlash(target.Path))
			} else {
				opts, err = req.options(template, "")
				dest = filepath.Join(spec.Output, filepath.FromSlash(target.Path),
					outputName(templateStem(filepath.FromSlash(target.Rest)), opts.Format))
			}
			if err != nil {
				return err
			}
			if other, ok := writers[dest]; ok {
				return fmt.Errorf("%s and %s would both write %s", other, template, dest)
			}
			writers[dest] = template
			out, err := req.render(template, opts)
			if err != nil {
				return err
			}
			if j, ok := written[dest]; ok {
				all[j].out = out
				continue
			}
			written[dest] = len(all)
			all = append(all, rendered{dest, out})
		}
	}
	if err := spec.CheckRules(); err != nil {
		return err
	}
	return writeAll(all)
}

// packageAt reads the yak.pkg at the root of dir, which a spec file's
// package must have.
func packageAt(dir string) (*module.Package, error) {
	src, err := os.ReadFile(filepath.Join(dir, module.PkgFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s is not a package: it has no %s", dir, module.PkgFile)
	}
	if err != nil {
		return nil, err
	}
	return module.ParsePackage(filepath.Join(dir, module.PkgFile), src)
}
