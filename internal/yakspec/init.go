package yakspec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultFile is the spec file "yak spec init" writes when it is given none.
const DefaultFile = "release" + Ext

// example is the spec file Init writes.
const example = `# The packages "yak spec render" renders, and how.
spec: "yakspec/v1"

# Every output goes beneath this directory, relative to this file; it
# defaults to "build".
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
	if err := checkPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
