package yakspec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/daluz/yak/internal/module"
)

// DefaultFile is the spec file Init writes when it is given none.
const DefaultFile = "release" + Ext

// example is the spec file Init writes.
const example = `# The packages "yak spec render" renders, and how.
spec: "yakspec/v1"

# Every output goes beneath this directory, relative to this file.
output: "build"

# The context every package is rendered against. A package's own context
# merges over it.
context:
  env: "dev"

# Where the outputs go. A key covers a template, a directory ending in "/",
# or "./" for the whole package; its value is a file, a directory ending in
# "/", or null to leave them out. The longest key that covers a template
# decides, and a package's own rules replace the ones here with its keys.
# Without a rule, a package's outputs go beneath a directory named for it.
#
# rules:
#   "tests/": null

packages:
  # A directory relative to this file, or a git+https:// or oci:// package.
  - source: "web"
    # context:
    #   replicas: 2
    # rules:
    #   "./": "manifests/"
`

// Init writes an example spec file at path, which must end in Ext and not
// exist yet, outside any package.
func Init(path string) error {
	if !strings.HasSuffix(path, Ext) {
		return fmt.Errorf("%s is not a %s spec file", path, Ext)
	}
	dir := filepath.Dir(path)
	pkg, err := module.FindPackage(dir)
	if err != nil {
		return err
	}
	if pkg != nil {
		return fmt.Errorf("%s would be inside the package %s at %s; a spec file names packages, and cannot belong to one", path, pkg.Name, pkg.Root())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already exists", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString(example); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
