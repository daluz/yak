// Package module finds and loads the module an "import" names: a built-in
// module of the standard library, or a file module read from a template file.
//
// A package is a directory holding a yak.pkg. A file module's path resolves
// from the root of the package the importing file is in, or from the
// directory of the importing file when it is in none, unless the package's
// yak.pkg maps the path, or a prefix of it, onto one of its dependencies.
package module

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// Std is the first segment of every standard library module. It never
	// names a file, and a yak.pkg cannot map it.
	Std = "std"
	// PkgFile is the name of the file that makes a directory a package.
	PkgFile = "yak.pkg"
	// Ext is the extension of a template, a module that can be rendered.
	Ext = ".yak"
	// ExtLibrary is the extension of a library, a module that can only be
	// imported.
	ExtLibrary = ".yakl"
)

// Resolve returns the file that path names when it is imported by the file
// at from.
func Resolve(from string, path []string) (string, error) {
	dir, segs := filepath.Dir(from), path
	pkg, err := FindPackage(dir)
	if err != nil {
		return "", err
	}
	if pkg != nil {
		dir = pkg.Root()
		target, rest, err := pkg.resolve(path)
		switch {
		case err != nil:
			return "", err
		case target != "" && len(rest) == 0:
			return target, nil
		case target != "":
			dir, segs = target, rest
		}
	}
	return lookup(dir, segs, strings.Join(path, "."))
}

// ResolveFile returns the file that the quoted path names when it is
// imported by the file at from. Only a file in no package can import one,
// and the path must name a template or a library beneath from's directory.
func ResolveFile(from, path string) (string, error) {
	dir := filepath.Dir(from)
	pkg, err := FindPackage(dir)
	if err != nil {
		return "", err
	}
	if pkg != nil {
		return "", fmt.Errorf("%q is a file path, which a file inside the package %s at %s cannot import; import it by its dotted path or name it in %s",
			path, pkg.Name, pkg.Root(), PkgFile)
	}
	if !isModule(path) {
		return "", fmt.Errorf("%q names no module; a file path ends in %s or %s", path, Ext, ExtLibrary)
	}
	if !filepath.IsLocal(filepath.FromSlash(path)) {
		return "", fmt.Errorf("%q is not a path beneath %s", path, dir)
	}
	file := filepath.Join(dir, filepath.FromSlash(path))
	info, err := os.Stat(file)
	switch {
	case err != nil:
		return "", fmt.Errorf("no module %q: %s does not exist", path, file)
	case info.IsDir():
		return "", fmt.Errorf("%q is a directory; import one of the modules in it", path)
	}
	return file, nil
}

// lookup finds the module that segs names under dir: the directories of
// every segment but the last, and the template or library that the last one
// names. name is the whole path as the import wrote it.
func lookup(dir string, segs []string, name string) (string, error) {
	base := filepath.Join(append([]string{dir}, segs...)...)
	var found []string
	for _, ext := range []string{Ext, ExtLibrary} {
		if _, err := os.Stat(base + ext); err == nil {
			found = append(found, base+ext)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 2:
		return "", fmt.Errorf("%q names both %s and %s; keep only one of them", name, found[0], found[1])
	}
	if info, err := os.Stat(base); err == nil && info.IsDir() {
		return "", fmt.Errorf("%q is a directory; import one of the modules in it", name)
	}
	return "", fmt.Errorf("no module %q: neither %s nor %s exists", name, base+Ext, base+ExtLibrary)
}

// isModule reports whether path names a module's file rather than a
// directory.
func isModule(path string) bool {
	return strings.HasSuffix(path, Ext) || strings.HasSuffix(path, ExtLibrary)
}

// Package is a parsed yak.pkg.
type Package struct {
	// Path is the yak.pkg file, whose directory is the package root.
	Path    string
	Name    string
	Version string
	// Context is the package's context file as it was written, relative to
	// the package root, or empty when it has none.
	Context string
	// Dependencies maps a dotted module path onto the location it was
	// written as.
	Dependencies map[string]string
}

// Root is the directory of the package.
func (p *Package) Root() string { return filepath.Dir(p.Path) }

// ContextPath answers the package's context file, or the empty string when
// it has none.
func (p *Package) ContextPath() string {
	if p.Context == "" {
		return ""
	}
	return filepath.Join(p.Root(), filepath.FromSlash(p.Context))
}

// IsContext reports whether path names the package's context file.
func (p *Package) IsContext(path string) bool {
	if p.Context == "" {
		return false
	}
	want, err := filepath.Abs(p.ContextPath())
	if err != nil {
		return false
	}
	got, err := filepath.Abs(path)
	return err == nil && got == want
}

// FindPackage reads the nearest yak.pkg in dir or above it, and answers nil
// when there is none.
func FindPackage(dir string) (*Package, error) {
	for {
		path := filepath.Join(dir, PkgFile)
		src, err := os.ReadFile(path)
		if err == nil {
			return ParsePackage(path, src)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		parent := filepath.Join(dir, "..")
		here, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		above, err := filepath.Abs(parent)
		if err != nil {
			return nil, err
		}
		if here == above {
			return nil, nil
		}
		dir = parent
	}
}

// ParsePackage parses src as the yak.pkg at path.
func ParsePackage(path string, src []byte) (*Package, error) {
	var doc struct {
		Package struct {
			Name        string    `yaml:"name"`
			Version     string    `yaml:"version"`
			Description string    `yaml:"description"`
			Context     yaml.Node `yaml:"context"`
		} `yaml:"package"`
		Dependencies map[string]string `yaml:"dependencies"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Package.Name == "" {
		return nil, fmt.Errorf("%s: the package has no name; give it one as package.name", path)
	}
	if !IsName(doc.Package.Name) {
		return nil, fmt.Errorf("%s: %q cannot name a package; a package name is an identifier", path, doc.Package.Name)
	}
	context, err := contextFile(path, &doc.Package.Context)
	if err != nil {
		return nil, err
	}
	for key, target := range doc.Dependencies {
		segs := strings.Split(key, ".")
		for _, s := range segs {
			if !IsName(s) {
				return nil, fmt.Errorf("%s: %q is not a module path", path, key)
			}
		}
		if segs[0] == Std {
			return nil, fmt.Errorf("%s: %q is reserved for the standard library and cannot be mapped", path, key)
		}
		if target == "" {
			return nil, fmt.Errorf("%s: %q maps to nothing", path, key)
		}
	}
	return &Package{Path: path, Name: doc.Package.Name, Version: doc.Package.Version, Context: context,
		Dependencies: doc.Dependencies}, nil
}

// contextFile reads package.context from the yak.pkg at path, which names a
// template beneath the package root.
func contextFile(path string, n *yaml.Node) (string, error) {
	switch {
	case n.Kind == 0:
		return "", nil
	case n.Kind == yaml.SequenceNode:
		return "", fmt.Errorf("%s: package.context names one %s template that answers the context, not a list of context files", path, Ext)
	case n.Kind != yaml.ScalarNode || n.Tag != "!!str":
		return "", fmt.Errorf("%s: package.context must name a %s template", path, Ext)
	}
	c := n.Value
	if !filepath.IsLocal(filepath.FromSlash(c)) {
		return "", fmt.Errorf("%s: the context file %q is not a path beneath the package root", path, c)
	}
	if !strings.HasSuffix(c, Ext) {
		return "", fmt.Errorf("%s: the context file %q is not a %s template", path, c, Ext)
	}
	return c, nil
}

// IsName reports whether s can be written as one segment of an import path,
// which is an identifier.
func IsName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		letter := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// resolve applies the longest dependency whose key is a prefix of path. It
// answers the location the dependency maps to and the segments of path left
// below it, or an empty location when no dependency covers the path. A
// location that is a module has nothing left below it; one that is a package
// always does.
func (p *Package) resolve(path []string) (string, []string, error) {
	var key []string
	var target string
	for k, t := range p.Dependencies {
		segs := strings.Split(k, ".")
		if len(segs) > len(path) || len(segs) <= len(key) {
			continue
		}
		if strings.Join(path[:len(segs)], ".") == k {
			key, target = segs, t
		}
	}
	if key == nil {
		return "", nil, nil
	}
	name := strings.Join(key, ".")
	if IsRemote(target) {
		src, err := ParseSource(target)
		if err != nil {
			return "", nil, fmt.Errorf("%s maps %q to %w", p.Path, name, err)
		}
		vendor, dir, ok := vendored(p.Root(), src.Key)
		if !ok {
			return "", nil, fmt.Errorf("%s maps %q to %s, which is not vendored; run yak sync", p.Path, name, target)
		}
		if err := checkVendored(vendor, src, target); err != nil {
			return "", nil, fmt.Errorf("%s maps %q to %s, but %w", p.Path, name, target, err)
		}
		target = filepath.Join(dir, filepath.FromSlash(src.Dir))
	} else if !filepath.IsAbs(target) {
		target = filepath.Join(p.Root(), target)
	}
	rest := path[len(key):]
	if isModule(target) {
		if len(rest) > 0 {
			return "", nil, fmt.Errorf("%s maps %q to the module %s, so %q is not inside a package",
				p.Path, name, target, strings.Join(path, "."))
		}
		if _, err := os.Stat(target); err != nil {
			return "", nil, fmt.Errorf("%s maps %q to %s, which does not exist", p.Path, name, target)
		}
		return target, nil, nil
	}
	if _, err := os.Stat(filepath.Join(target, PkgFile)); err != nil {
		return "", nil, fmt.Errorf("%s maps %q to %s, which is not a package: it holds no %s",
			p.Path, name, target, PkgFile)
	}
	if len(rest) == 0 {
		return "", nil, fmt.Errorf("%s maps %q to the package %s; import one of the modules in it",
			p.Path, name, target)
	}
	return target, rest, nil
}
