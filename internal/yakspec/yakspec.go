// Package yakspec reads a spec file: a .yakspec file that names the packages
// "yak spec render" renders, the context each is rendered against, the
// options they are rendered with and where each template's output goes.
//
// A spec file is written in yak, but it is no module and belongs to no
// package. It is evaluated outside strict mode against an empty context, and
// what it renders is read back as plain data and checked against the shape
// its spec version gives it.
package yakspec

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/render"
)

const (
	// Ext is the extension of a spec file.
	Ext = ".yakspec"
	// Version is the one spec version there is, which a spec file that
	// names none is read as.
	Version = "yakspec/v1"
)

// Spec is a spec file as it was read.
type Spec struct {
	// Path is the spec file.
	Path string
	// Output is the directory every output path is relative to.
	Output             string
	Format             string
	KeepNullDocuments  bool
	SkipSchemaChecks   bool
	SkipPackageContext bool
	SkipRenderDeps     bool
	NonStrict          bool
	PlainHTTP          bool
	Packages           []*Package
	// rules are the top-level ones, which every package shares.
	rules []*module.Rule
}

// Package is one entry of packages.
type Package struct {
	// Source is the package's directory, joined to the spec file's, or
	// the reference of a remote package.
	Source string
	// Context is what the package is rendered against: its own context
	// merged over the top-level one.
	Context *eval.Object
	// own are the rules the package's entry gives.
	own []*module.Rule
	// rules are own followed by the top-level ones it does not override.
	rules []*module.Rule
	label string
}

// Target is where one template's output goes, with its Path relative to
// Output.
type Target = module.Target

// Target answers where the template at rel, a slash path relative to the
// root of the package called name, goes. The longest rule that covers it
// decides; without one it goes beneath a directory named for the package.
func (p *Package) Target(name, rel string) Target {
	if t, ok := module.Place(p.rules, rel); ok {
		return t
	}
	return Target{Path: name, Rest: rel}
}

// CheckRules refuses a rule that covered no template, once every template
// has been given its Target: a top-level rule must cover one in some
// package, and a package's own rule one in that package.
func (s *Spec) CheckRules() error {
	for _, r := range s.rules {
		if !r.Used() {
			return fmt.Errorf("%s: rules: %s covers no template in any package, or every package overrides it", s.Path, r.Key)
		}
	}
	for _, p := range s.Packages {
		for _, r := range p.own {
			if !r.Used() {
				return fmt.Errorf("%s: %s.rules: %s covers no template of %s", s.Path, p.label, r.Key, p.Source)
			}
		}
	}
	return nil
}

// Load reads the spec file at path, which must end in Ext.
func Load(path string) (*Spec, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec file: %w", err)
	}
	v, err := evaluate(path, src)
	if err != nil {
		return nil, err
	}
	return decode(path, v)
}

// checkPath refuses a spec file path that does not end in Ext, or that
// lies inside a package.
func checkPath(path string) error {
	if !strings.HasSuffix(path, Ext) {
		return fmt.Errorf("%s is not a %s spec file", path, Ext)
	}
	pkg, err := module.FindPackage(filepath.Dir(path))
	if err != nil {
		return err
	}
	if pkg != nil {
		return fmt.Errorf("%s is inside the package %s at %s; a spec file names packages, and cannot belong to one", path, pkg.Name, pkg.Root())
	}
	return nil
}

// evaluate renders the spec file and reads what it renders back as data.
func evaluate(path string, src []byte) (eval.Value, error) {
	stream, err := parser.Parse(path, src)
	if err != nil {
		return nil, err
	}
	if len(stream.Docs) != 1 {
		return nil, fmt.Errorf("%s: a spec file holds one document, not %d", path, len(stream.Docs))
	}
	if m, ok := stream.Docs[0].Body.(*ast.Mapping); ok {
		for _, b := range m.Binds {
			if _, ok := b.Value.(*ast.Schema); ok && b.Name == ast.SchemaContext {
				return nil, fmt.Errorf("%s: a spec file has no context, so it cannot declare a schema for it", b.Pos())
			}
		}
	}
	workdir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("finding the working directory: %w", err)
	}
	run := eval.Run{FilePath: path, Workdir: workdir, NonStrict: true}
	var buf bytes.Buffer
	if err := engine.Render(run, src, eval.NewObject(), &buf, render.DefaultOptions()); err != nil {
		return nil, err
	}
	return ctxfile.ValueYAML(path, buf.Bytes())
}

// decoder checks the value a spec file renders, naming the file in what it
// reports.
type decoder struct{ path string }

func (d decoder) errorf(key, format string, args ...any) error {
	return fmt.Errorf("%s: %s %s", d.path, key, fmt.Sprintf(format, args...))
}

func (d decoder) str(key string, v eval.Value) (string, error) {
	s, ok := v.(eval.String)
	if !ok {
		return "", d.errorf(key, "must be a string, found %s", v.TypeName())
	}
	return string(s), nil
}

func (d decoder) boolean(key string, v eval.Value) (bool, error) {
	b, ok := v.(eval.Bool)
	if !ok {
		return false, d.errorf(key, "must be true or false, found %s", v.TypeName())
	}
	return bool(b), nil
}

func (d decoder) mapping(key string, v eval.Value) (*eval.Object, error) {
	o, ok := v.(*eval.Object)
	if !ok {
		return nil, d.errorf(key, "must be a mapping, found %s", v.TypeName())
	}
	return o, nil
}

func decode(file string, v eval.Value) (*Spec, error) {
	d := decoder{path: file}
	top, ok := v.(*eval.Object)
	if !ok {
		return nil, fmt.Errorf("%s: a spec file must hold a mapping, found %s", file, v.TypeName())
	}
	s := &Spec{Path: file, Output: "build"}
	context := eval.NewObject()
	for _, f := range top.Fields() {
		name := f.Name
		v, err := f.Value.Value()
		if err != nil {
			return nil, err
		}
		switch name {
		case "spec":
			version, err := d.str(name, v)
			if err != nil {
				return nil, err
			}
			if version != Version {
				return nil, d.errorf(name, "%q is not a version this yak reads; it reads %q", version, Version)
			}
		case "output":
			if s.Output, err = d.str(name, v); err != nil {
				return nil, err
			}
			if s.Output == "" {
				return nil, d.errorf(name, "names no directory")
			}
		case "format":
			if s.Format, err = d.str(name, v); err != nil {
				return nil, err
			}
			if _, err := render.ParseFormat(s.Format); err != nil {
				return nil, d.errorf(name, "%v", err)
			}
		case "keepNullDocuments":
			s.KeepNullDocuments, err = d.boolean(name, v)
		case "skipSchemaChecks":
			s.SkipSchemaChecks, err = d.boolean(name, v)
		case "skipPackageContext":
			s.SkipPackageContext, err = d.boolean(name, v)
		case "skipRenderDeps":
			s.SkipRenderDeps, err = d.boolean(name, v)
		case "plainHttp":
			s.PlainHTTP, err = d.boolean(name, v)
		case "strict":
			var strict bool
			strict, err = d.boolean(name, v)
			s.NonStrict = !strict
		case "context":
			context, err = d.mapping(name, v)
		case "rules":
			s.rules, err = d.rules(name, v)
		case "packages":
			s.Packages, err = d.packages(name, v)
		default:
			return nil, d.errorf(name, "is not a key of a spec file; it takes spec, output, format, strict, "+
				"keepNullDocuments, skipSchemaChecks, skipPackageContext, skipRenderDeps, plainHttp, context, rules and packages")
		}
		if err != nil {
			return nil, err
		}
	}
	if len(s.Packages) == 0 {
		return nil, d.errorf("packages", "names nothing to render")
	}
	dir := filepath.Dir(file)
	if !filepath.IsAbs(s.Output) {
		s.Output = filepath.Join(dir, s.Output)
	}
	for _, p := range s.Packages {
		if !module.IsRemote(p.Source) && !filepath.IsAbs(p.Source) {
			p.Source = filepath.Join(dir, p.Source)
		}
		var err error
		if p.Context, err = eval.Merge(context, p.Context); err != nil {
			return nil, err
		}
		p.rules = append([]*module.Rule(nil), p.own...)
		for _, r := range s.rules {
			if !slices.ContainsFunc(p.own, func(o *module.Rule) bool { return o.Key == r.Key }) {
				p.rules = append(p.rules, r)
			}
		}
	}
	return s, nil
}

func (d decoder) packages(key string, v eval.Value) ([]*Package, error) {
	a, ok := v.(*eval.Array)
	if !ok {
		return nil, d.errorf(key, "must be a sequence of packages, found %s", v.TypeName())
	}
	var pkgs []*Package
	for i, item := range a.Items() {
		label := fmt.Sprintf("%s[%d]", key, i)
		v, err := item.Value.Value()
		if err != nil {
			return nil, err
		}
		o, err := d.mapping(label, v)
		if err != nil {
			return nil, err
		}
		p := &Package{Context: eval.NewObject(), label: label}
		for _, f := range o.Fields() {
			v, err := f.Value.Value()
			if err != nil {
				return nil, err
			}
			k := label + "." + f.Name
			switch f.Name {
			case "source":
				p.Source, err = d.str(k, v)
				if err == nil && p.Source == "" {
					err = d.errorf(k, "names no package")
				}
			case "context":
				p.Context, err = d.mapping(k, v)
			case "rules":
				p.own, err = d.rules(k, v)
			default:
				err = d.errorf(k, "is not a key of a package; it takes source, context and rules")
			}
			if err != nil {
				return nil, err
			}
		}
		if p.Source == "" {
			return nil, d.errorf(label, "has no source; name the package's directory or remote reference")
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func (d decoder) rules(key string, v eval.Value) ([]*module.Rule, error) {
	o, err := d.mapping(key, v)
	if err != nil {
		return nil, err
	}
	var rules []*module.Rule
	for _, f := range o.Fields() {
		v, err := f.Value.Value()
		if err != nil {
			return nil, err
		}
		k := key + ": " + f.Name
		var r *module.Rule
		switch to := v.(type) {
		case eval.Null:
			r, err = module.NewRule(f.Name, "", true)
		case eval.String:
			r, err = module.NewRule(f.Name, string(to), false)
		default:
			return nil, d.errorf(k, "must be sent to a path or to null, found %s", to.TypeName())
		}
		if err != nil {
			return nil, d.errorf(k, "%v", err)
		}
		rules = append(rules, r)
	}
	return rules, nil
}
