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
	// Render maps the dependencies that render along with the package onto
	// how they render.
	Render map[string]*Render
	// Rules send the package's own templates and included files where they
	// go, relative to the package's output directory.
	Rules []*Rule
	// Includes are the files, and the directories ending in "/", that are
	// copied to the output as they are, as slash paths relative to the
	// package root.
	Includes []string
}

// Render is how a dependency renders when the package that depends on it is
// rendered.
type Render struct {
	// Rules send the dependency's templates where they go, relative to the
	// output directory of the package whose yak.pkg writes them.
	Rules []*Rule
	// All renders what the dependency's own yak.pkg renders, as it renders
	// it.
	All bool
	// Deps, when All is not set, maps the dependency's own dependencies that
	// render along with it onto how they render.
	Deps map[string]*Render
	// Label names the yak.pkg and the key it is written at.
	Label string
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
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		if pkg, err := ReadPackage(dir); pkg != nil || err != nil {
			return pkg, err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return nil, nil
		}
		abs, dir = parent, filepath.Join(dir, "..")
	}
}

// ReadPackage reads the yak.pkg in dir, and answers nil when there is none.
func ReadPackage(dir string) (*Package, error) {
	path := filepath.Join(dir, PkgFile)
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ParsePackage(path, src)
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
		Dependencies map[string]yaml.Node `yaml:"dependencies"`
		Render       yaml.Node            `yaml:"render"`
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
	pkg := &Package{Path: path, Name: doc.Package.Name, Version: doc.Package.Version, Context: context,
		Dependencies: map[string]string{}, Render: map[string]*Render{}}
	for key, n := range doc.Dependencies {
		if err := modulePath(key); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if strings.Split(key, ".")[0] == Std {
			return nil, fmt.Errorf("%s: %q is reserved for the standard library and cannot be mapped", path, key)
		}
		target, render, err := dependency(path, key, &n)
		if err != nil {
			return nil, err
		}
		pkg.Dependencies[key] = target
		if render != nil {
			pkg.Render[key] = render
		}
	}
	if err := ownRender(path, &doc.Render, pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

// ownRender reads the top-level render of the yak.pkg at path into pkg: a
// mapping of rules and includes.
func ownRender(path string, n *yaml.Node, pkg *Package) error {
	if n.Kind == 0 {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: render must be a mapping of rules and includes", path)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		var err error
		switch k {
		case "rules":
			pkg.Rules, err = rules(path, "render.rules", v)
		case "includes":
			pkg.Includes, err = includes(path, v)
		default:
			err = fmt.Errorf("%s: render.%s is not a key of render; it takes rules and includes", path, k)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// includes reads render.includes from the yak.pkg at path: a sequence of
// files and directories ending in "/" beneath the package root.
func includes(path string, n *yaml.Node) ([]string, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: render.includes must be a sequence of files and directories", path)
	}
	var paths []string
	for _, item := range n.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return nil, fmt.Errorf("%s: render.includes must name files and directories", path)
		}
		p := trimRoot(item.Value)
		if !isLocalPath(strings.TrimSuffix(p, "/")) {
			return nil, fmt.Errorf("%s: render.includes: %q is not a path beneath the package root", path, item.Value)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func modulePath(key string) error {
	for _, s := range strings.Split(key, ".") {
		if !IsName(s) {
			return fmt.Errorf("%q is not a module path", key)
		}
	}
	return nil
}

// dependency reads the dependency key of the yak.pkg at path: the location
// it is written as, alone or as the source of a mapping that also says how
// it renders.
func dependency(path, key string, n *yaml.Node) (string, *Render, error) {
	label := "dependencies." + key
	if n.Kind == yaml.ScalarNode {
		if n.Tag == "!!null" || n.Value == "" {
			return "", nil, fmt.Errorf("%s: %q maps to nothing", path, key)
		}
		if n.Tag != "!!str" {
			return "", nil, fmt.Errorf("%s: %s must name a path or a remote package", path, label)
		}
		return n.Value, nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return "", nil, fmt.Errorf("%s: %s must name a path or a remote package, or be a mapping of source and render", path, label)
	}
	var target string
	var render *Render
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		var err error
		switch k {
		case "source":
			if v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
				return "", nil, fmt.Errorf("%s: %s.source must name a path or a remote package", path, label)
			}
			target = v.Value
		case "render":
			render, err = renderValue(path, label+".render", v)
		default:
			err = fmt.Errorf("%s: %s.%s is not a key of a dependency; it takes source and render", path, label, k)
		}
		if err != nil {
			return "", nil, err
		}
	}
	if target == "" {
		return "", nil, fmt.Errorf("%s: %s has no source; name a path or a remote package", path, label)
	}
	switch {
	case render == nil:
	case strings.HasSuffix(target, ExtLibrary):
		return "", nil, fmt.Errorf("%s: %s: %s is a library, which can be imported but not rendered", path, label, target)
	case strings.HasSuffix(target, Ext) && (render.All || render.Deps != nil):
		return "", nil, fmt.Errorf("%s: %s.render.transitive: %s is a module, which has no dependencies", path, label, target)
	}
	return target, render, nil
}

// renderValue reads how a dependency renders, written at label: true,
// false, which answers nil, or a mapping of rules and transitive.
func renderValue(path, label string, n *yaml.Node) (*Render, error) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!bool" {
		var on bool
		if err := n.Decode(&on); err != nil || !on {
			return nil, err
		}
		return &Render{Label: path + ": " + label}, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: %s must be true, false or a mapping of rules and transitive", path, label)
	}
	r := &Render{Label: path + ": " + label}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		var err error
		switch k {
		case "rules":
			r.Rules, err = rules(path, label+".rules", v)
		case "transitive":
			err = transitive(path, label+".transitive", v, r)
		default:
			err = fmt.Errorf("%s: %s.%s is not a key of render; it takes rules and transitive", path, label, k)
		}
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

// transitive reads which of a dependency's own dependencies render along
// with it into r: true for what its yak.pkg renders, false for none, or a
// sequence or a mapping of their names, a mapping saying how each renders.
func transitive(path, label string, n *yaml.Node, r *Render) error {
	name := func(n *yaml.Node) (string, error) {
		if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
			return "", fmt.Errorf("%s: %s must name dependencies", path, label)
		}
		if err := modulePath(n.Value); err != nil {
			return "", fmt.Errorf("%s: %s: %w", path, label, err)
		}
		if _, ok := r.Deps[n.Value]; ok {
			return "", fmt.Errorf("%s: %s names %s twice", path, label, n.Value)
		}
		return n.Value, nil
	}
	switch {
	case n.Kind == yaml.ScalarNode && n.Tag == "!!bool":
		return n.Decode(&r.All)
	case n.Kind == yaml.SequenceNode:
		r.Deps = map[string]*Render{}
		for _, item := range n.Content {
			dep, err := name(item)
			if err != nil {
				return err
			}
			r.Deps[dep] = &Render{Label: path + ": " + label + "." + dep}
		}
		return nil
	case n.Kind == yaml.MappingNode:
		r.Deps = map[string]*Render{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			dep, err := name(n.Content[i])
			if err != nil {
				return err
			}
			if r.Deps[dep], err = renderValue(path, label+"."+dep, n.Content[i+1]); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("%s: %s must be true, false, a sequence of dependencies or a mapping of them onto how they render", path, label)
}

// rules reads the rules written at label.
func rules(path, label string, n *yaml.Node) ([]*Rule, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: %s must be a mapping", path, label)
	}
	var rules []*Rule
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		var r *Rule
		var err error
		switch {
		case v.Kind == yaml.ScalarNode && v.Tag == "!!null":
			r, err = NewRule(k, "", true)
		case v.Kind == yaml.ScalarNode && v.Tag == "!!str":
			r, err = NewRule(k, v.Value, false)
		default:
			err = errors.New("must be sent to a path or to null")
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %s %w", path, label, k, err)
		}
		rules = append(rules, r)
	}
	return rules, nil
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
	for k := range p.Dependencies {
		segs := strings.Split(k, ".")
		if len(segs) > len(path) || len(segs) <= len(key) {
			continue
		}
		if strings.Join(path[:len(segs)], ".") == k {
			key = segs
		}
	}
	if key == nil {
		return "", nil, nil
	}
	name := strings.Join(key, ".")
	rest := path[len(key):]
	target, err := p.Locate(name)
	if err != nil {
		return "", nil, err
	}
	if isModule(target) && len(rest) > 0 {
		return "", nil, fmt.Errorf("%s maps %q to the module %s, so %q is not inside a package",
			p.Path, name, target, strings.Join(path, "."))
	}
	if !isModule(target) && len(rest) == 0 {
		return "", nil, fmt.Errorf("%s maps %q to the package %s; import one of the modules in it",
			p.Path, name, target)
	}
	return target, rest, nil
}

// Locate answers the module's file or the package's directory that the
// dependency name of p maps to, in the vendor directory when it is remote.
func (p *Package) Locate(name string) (string, error) {
	target, ok := p.Dependencies[name]
	if !ok {
		return "", fmt.Errorf("%s has no dependency %q", p.Path, name)
	}
	if IsRemote(target) {
		src, err := ParseSource(target)
		if err != nil {
			return "", fmt.Errorf("%s maps %q to %w", p.Path, name, err)
		}
		vendor, dir, ok := vendored(p.Root(), src.Key)
		if !ok {
			return "", fmt.Errorf("%s maps %q to %s, which is not vendored; run yak sync", p.Path, name, target)
		}
		if err := checkVendored(vendor, src, target); err != nil {
			return "", fmt.Errorf("%s maps %q to %s, but %w", p.Path, name, target, err)
		}
		target = filepath.Join(dir, filepath.FromSlash(src.Dir))
	} else if !filepath.IsAbs(target) {
		target = filepath.Join(p.Root(), target)
	}
	if isModule(target) {
		if _, err := os.Stat(target); err != nil {
			return "", fmt.Errorf("%s maps %q to %s, which does not exist", p.Path, name, target)
		}
		return target, nil
	}
	if _, err := os.Stat(filepath.Join(target, PkgFile)); err != nil {
		return "", fmt.Errorf("%s maps %q to %s, which is not a package: it holds no %s",
			p.Path, name, target, PkgFile)
	}
	return target, nil
}
