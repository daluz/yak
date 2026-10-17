package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/daluz/yak/internal/deps"
	"github.com/daluz/yak/internal/module"
)

func newPackageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "package",
		Aliases: []string{"pkg"},
		Short:   "Manage the package in the current directory",
		Long: "Manage the package in the current directory: the one whose yak.pkg is\n" +
			"there or in the nearest directory above it.",
	}
	cmd.AddCommand(newAddCommand(), newRemoveCommand(), newSyncCommand())
	return cmd
}

// shortcut makes cmd a top-level command that does what "yak package" does
// under the same name.
func shortcut(cmd *cobra.Command) *cobra.Command {
	cmd.Short += " (yak package " + cmd.Name() + ")"
	return cmd
}

// currentPackage reads the package the current directory is in.
func currentPackage() (*module.Package, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	pkg, err := module.FindPackage(wd)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, fmt.Errorf("no %s in %s or any directory above it", module.PkgFile, wd)
	}
	return pkg, nil
}

func newAddCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add SOURCE",
		Short: "Add a dependency to the package",
		Long: "Add a dependency to the package's yak.pkg.\n\n" +
			"SOURCE is a local path, to a package or to a module, or a remote package\n" +
			"written as git+https://HOST/REPOSITORY[//DIR][@REF], or with git+ssh:// or\n" +
			"git+file://. The dependency is named after the package, or after the\n" +
			"module's file, unless --name gives it a name. A remote package is vendored\n" +
			"and pinned in yak.lock at once.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := currentPackage()
			if err != nil {
				return err
			}
			return deps.Add(pkg, args[0], name, deps.Options{Log: cmd.ErrOrStderr()})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "import path to add the dependency under")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a dependency from the package",
		Long: "Remove a dependency from the package's yak.pkg. A remote one leaves\n" +
			"the vendor directory and yak.lock as well.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := currentPackage()
			if err != nil {
				return err
			}
			return deps.Remove(pkg, args[0], deps.Options{Log: cmd.ErrOrStderr()})
		},
	}
}

func newSyncCommand() *cobra.Command {
	var update bool
	cmd := &cobra.Command{
		Use:   "sync [--update [NAME]...]",
		Short: "Vendor the package's remote dependencies",
		Long: "Vendor every remote package the package depends on, directly or through\n" +
			"another, into vendor/ beneath its root, at the commits yak.lock pins, and\n" +
			"rewrite yak.lock. A dependency yak.lock does not pin yet, or whose ref\n" +
			"changed, is fetched at the commit its ref names now.\n\n" +
			"--update resolves the refs of the dependencies it names again, or of\n" +
			"every one when it names none.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && !update {
				return fmt.Errorf("dependencies are named only with --update")
			}
			pkg, err := currentPackage()
			if err != nil {
				return err
			}
			return deps.Sync(pkg, deps.Options{
				Update:    args,
				UpdateAll: update && len(args) == 0,
				Log:       cmd.ErrOrStderr(),
			})
		},
	}
	cmd.Flags().BoolVar(&update, "update", false, "resolve refs again instead of reading yak.lock")
	return cmd
}
