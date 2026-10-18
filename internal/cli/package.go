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
	cmd.AddCommand(newInitCommand(), newAddCommand(), newRemoveCommand(), newSyncCommand(),
		newBuildCommand(), newPushCommand())
	return cmd
}

func newInitCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init [DIR]",
		Short: "Make a directory a package",
		Long: "Make DIR, or the current directory, a package: write its yak.pkg, an\n" +
			"app.yak to start from unless there is one, and a .gitignore that leaves\n" +
			"out vendor/ and .yak/. The package is named after the directory unless\n" +
			"--name gives it a name.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return deps.Init(dir, name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "name of the package")
	return cmd
}

func newBuildCommand() *cobra.Command {
	var opts deps.BuildOptions
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the package as an OCI artifact",
		Long: "Pack the package's files, but for hidden ones and vendor/, into an OCI\n" +
			"artifact tagged with package.version, in the OCI image layout at\n" +
			".yak/oci beneath the package root, where yak package push finds it.\n" +
			"--output writes the layout somewhere else instead: a directory, or a\n" +
			"tar archive when it ends in .tar.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := currentPackage()
			if err != nil {
				return err
			}
			opts.Log = cmd.ErrOrStderr()
			return deps.Build(pkg, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Tag, "tag", "", "tag the build with this rather than package.version")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "write the OCI image layout to this directory or .tar")
	return cmd
}

func newPushCommand() *cobra.Command {
	var plainHTTP bool
	cmd := &cobra.Command{
		Use:   "push oci://REGISTRY/REPOSITORY[:TAG]",
		Short: "Push the package's build to an OCI registry",
		Long: "Upload the build that yak package build wrote to .yak/oci. The tag picks\n" +
			"the build and names the upload, and is package.version unless the\n" +
			"reference gives one. It logs in with what podman login or docker login\n" +
			"saved: $REGISTRY_AUTH_FILE, then Podman's auth.json, then Docker's\n" +
			"config.json.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := currentPackage()
			if err != nil {
				return err
			}
			return deps.Push(pkg, args[0], deps.Options{Log: cmd.ErrOrStderr(), PlainHTTP: plainHTTP})
		},
	}
	plainHTTPFlag(cmd, &plainHTTP)
	return cmd
}

func plainHTTPFlag(cmd *cobra.Command, v *bool) {
	cmd.Flags().BoolVar(v, "plain-http", false, "talk to OCI registries over HTTP rather than HTTPS")
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
	var plainHTTP bool
	cmd := &cobra.Command{
		Use:   "add SOURCE",
		Short: "Add a dependency to the package",
		Long: "Add a dependency to the package's yak.pkg.\n\n" +
			"SOURCE is a local path, to a package or to a module, or a remote package\n" +
			"written as git+https://HOST/REPOSITORY[//DIR][@REF], with git+ssh:// or\n" +
			"git+file://, or as oci://REGISTRY/REPOSITORY:TAG. The dependency is named\n" +
			"after the package, or after the module's file, unless --name gives it a\n" +
			"name. A remote package is vendored and pinned in yak.lock at once.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := currentPackage()
			if err != nil {
				return err
			}
			return deps.Add(pkg, args[0], name, deps.Options{Log: cmd.ErrOrStderr(), PlainHTTP: plainHTTP})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "import path to add the dependency under")
	plainHTTPFlag(cmd, &plainHTTP)
	return cmd
}

func newRemoveCommand() *cobra.Command {
	var plainHTTP bool
	cmd := &cobra.Command{
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
			return deps.Remove(pkg, args[0], deps.Options{Log: cmd.ErrOrStderr(), PlainHTTP: plainHTTP})
		},
	}
	plainHTTPFlag(cmd, &plainHTTP)
	return cmd
}

func newSyncCommand() *cobra.Command {
	var update, plainHTTP bool
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
				PlainHTTP: plainHTTP,
			})
		},
	}
	cmd.Flags().BoolVar(&update, "update", false, "resolve refs again instead of reading yak.lock")
	plainHTTPFlag(cmd, &plainHTTP)
	return cmd
}
