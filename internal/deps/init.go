package deps

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/daluz/yak/internal/module"
)

// ignored are the lines Init makes sure the .gitignore of a package holds.
var ignored = []string{"/" + module.VendorDir + "/", "/.yak/"}

// Init makes dir a package named name, or else after dir: it writes its
// yak.pkg, a first template unless there is one, and a .gitignore that
// leaves out what sync and build write.
func Init(dir, name string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	if !module.IsName(name) {
		return fmt.Errorf("%q cannot name a package, which takes an identifier; give a name with --name", name)
	}
	pkgfile := filepath.Join(dir, module.PkgFile)
	if _, err := os.Stat(pkgfile); err == nil {
		return fmt.Errorf("%s already exists", pkgfile)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src := fmt.Sprintf("package:\n  name: %s\n  version: 0.1.0\ndependencies: {}\n", name)
	if err := os.WriteFile(pkgfile, []byte(src), 0o644); err != nil {
		return err
	}
	app := filepath.Join(dir, "app"+module.Ext)
	if _, err := os.Stat(app); errors.Is(err, fs.ErrNotExist) {
		src := fmt.Sprintf("## Render the package with: yak render . --output-dir build\napp: %q\n", name)
		if err := os.WriteFile(app, []byte(src), 0o644); err != nil {
			return err
		}
	}
	return ignore(filepath.Join(dir, ".gitignore"))
}

// ignore adds the lines of ignored that the .gitignore at path lacks.
func ignore(path string) error {
	src, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	have := map[string]bool{}
	for line := range strings.SplitSeq(string(src), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	text := string(src)
	for _, line := range ignored {
		if have[line] {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += line + "\n"
	}
	return os.WriteFile(path, []byte(text), 0o644)
}
