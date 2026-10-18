// Package deps manages the dependencies of a package: it edits the
// dependencies of its yak.pkg, fetches the remote packages they name into
// its vendor directory, and pins what it fetched in its yak.lock.
//
// Vendoring is flat: the root package vendors every remote package it
// reaches, its own dependencies' included, once, under the key of the
// repository it comes from. A repository can be checked out at only one ref.
package deps

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/module"
)

// Options configure Sync, and the sync that Add and Remove run.
type Options struct {
	// Update names dependencies of the root package whose refs are resolved
	// again rather than read from yak.lock.
	Update []string
	// UpdateAll resolves the ref of every dependency again.
	UpdateAll bool
	// Log receives a line for every repository fetched.
	Log io.Writer
	// PlainHTTP talks to OCI registries over HTTP rather than HTTPS.
	PlainHTTP bool
}

// Sync vendors every remote package that pkg depends on, directly or
// through another, as yak.lock pins it, and rewrites yak.lock.
func Sync(pkg *module.Package, opts Options) error {
	s, err := newSyncer(pkg, opts)
	if err != nil {
		return err
	}
	defer s.close()
	return s.sync(pkg)
}

// Add adds source, a path or a remote package, to the dependencies of pkg
// under name, or else under the name the dependency's own yak.pkg gives it.
// A remote one is vendored, and pkg synced.
func Add(pkg *module.Package, source, name string, opts Options) error {
	if !module.IsRemote(source) {
		abs, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("%s does not exist", source)
		}
		if name, err = dependencyName(abs, source, name); err != nil {
			return err
		}
		root, err := filepath.Abs(pkg.Root())
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		_, content, err := addDependency(pkg, name, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		return os.WriteFile(pkg.Path, content, 0o644)
	}
	src, err := module.ParseSource(source)
	if err != nil {
		return err
	}
	s, err := newSyncer(pkg, opts)
	if err != nil {
		return err
	}
	defer s.close()
	if _, err := s.vendor(dependency{target: source, src: src, from: pkg.Path}); err != nil {
		return err
	}
	dir := filepath.Join(s.vendorPath(src.Key), filepath.FromSlash(src.Dir))
	if name, err = dependencyName(dir, source, name); err != nil {
		return err
	}
	updated, content, err := addDependency(pkg, name, source)
	if err != nil {
		return err
	}
	if err := s.sync(updated); err != nil {
		return err
	}
	return os.WriteFile(pkg.Path, content, 0o644)
}

// Remove removes the dependency name from pkg, and syncs pkg when it was a
// remote one, so that its vendored copy goes too.
func Remove(pkg *module.Package, name string, opts Options) error {
	target, ok := pkg.Dependencies[name]
	if !ok {
		return fmt.Errorf("%s has no dependency %q", pkg.Path, name)
	}
	updated, content, err := editDependencies(pkg, func(deps *yaml.Node) {
		for i := 0; i+1 < len(deps.Content); i += 2 {
			if deps.Content[i].Value == name {
				deps.Content = append(deps.Content[:i], deps.Content[i+2:]...)
				return
			}
		}
	})
	if err != nil {
		return err
	}
	if module.IsRemote(target) {
		if err := Sync(updated, opts); err != nil {
			return err
		}
	}
	return os.WriteFile(pkg.Path, content, 0o644)
}

// Fetch writes the remote package src names beneath dir and syncs it, and
// answers the directory that holds it.
func Fetch(src module.Source, dir string, opts Options) (string, error) {
	clone := filepath.Join(dir, "repository")
	if src.OCI {
		if _, err := pull(src, src.Ref, opts.PlainHTTP, clone); err != nil {
			return "", err
		}
	} else if _, err := checkout(src, src.Ref, clone); err != nil {
		return "", err
	}
	root := filepath.Join(clone, filepath.FromSlash(src.Dir))
	pkg, err := readPackage(root)
	if err != nil || pkg == nil {
		return root, err
	}
	return root, Sync(pkg, opts)
}

// dependency is a remote dependency as one yak.pkg writes it.
type dependency struct {
	target string
	src    module.Source
	// from is the yak.pkg that depends on it.
	from string
}

type syncer struct {
	root   string
	locked map[string]module.LockEntry
	// update holds the keys of the repositories whose refs are resolved
	// again; all resolves every one again.
	update map[string]bool
	all    bool
	// done holds the repositories vendored so far, by key.
	done      map[string]module.LockEntry
	log       io.Writer
	tmp       string
	plainHTTP bool
}

func newSyncer(pkg *module.Package, opts Options) (*syncer, error) {
	locked, err := module.ReadLock(filepath.Join(pkg.Root(), module.LockFile))
	if err != nil {
		return nil, err
	}
	s := &syncer{
		root:      pkg.Root(),
		locked:    locked,
		update:    map[string]bool{},
		all:       opts.UpdateAll,
		done:      map[string]module.LockEntry{},
		log:       opts.Log,
		plainHTTP: opts.PlainHTTP,
	}
	if s.log == nil {
		s.log = io.Discard
	}
	for _, name := range opts.Update {
		target, ok := pkg.Dependencies[name]
		if !ok {
			return nil, fmt.Errorf("%s has no dependency %q", pkg.Path, name)
		}
		if !module.IsRemote(target) {
			return nil, fmt.Errorf("%q is the local path %s, which has nothing to update", name, target)
		}
		src, err := module.ParseSource(target)
		if err != nil {
			return nil, err
		}
		s.update[src.Key] = true
	}
	if s.tmp, err = os.MkdirTemp("", "yak-sync-"); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *syncer) close() { os.RemoveAll(s.tmp) }

func (s *syncer) vendorPath(key string) string {
	return filepath.Join(s.root, module.VendorDir, filepath.FromSlash(key))
}

// sync vendors what pkg depends on, then what that depends on in turn,
// prunes the vendor directory and writes yak.lock.
func (s *syncer) sync(pkg *module.Package) error {
	queue, err := remoteDependencies(pkg)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		d := queue[0]
		queue = queue[1:]
		if _, err := s.vendor(d); err != nil {
			return err
		}
		dir := filepath.Join(s.vendorPath(d.src.Key), filepath.FromSlash(d.src.Dir))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		sub, err := readPackage(dir)
		if err != nil {
			return err
		}
		if sub == nil {
			return fmt.Errorf("%s depends on %s, which is not a package: it holds no %s", d.from, d.target, module.PkgFile)
		}
		more, err := remoteDependencies(sub)
		if err != nil {
			return err
		}
		queue = append(queue, more...)
	}
	if err := prune(filepath.Join(s.root, module.VendorDir), s.done); err != nil {
		return err
	}
	return writeLock(filepath.Join(s.root, module.LockFile), s.done)
}

// vendor makes the vendor directory hold the repository d names: at the
// commit yak.lock pins, unless its ref changed or is being updated, and
// otherwise at the commit its ref names now.
func (s *syncer) vendor(d dependency) (module.LockEntry, error) {
	src := d.src
	if e, ok := s.done[src.Key]; ok {
		if e.Ref != src.Ref {
			return e, fmt.Errorf("%s depends on %s at %s, but it is vendored at %s; depend on one ref of a repository",
				d.from, src.Key, refName(src.Ref), refName(e.Ref))
		}
		return e, nil
	}
	for key := range s.done {
		if strings.HasPrefix(key, src.Key+"/") || strings.HasPrefix(src.Key, key+"/") {
			return module.LockEntry{}, fmt.Errorf("cannot vendor both %s and %s, as one would be inside the other", key, src.Key)
		}
	}
	dest := s.vendorPath(src.Key)
	e, pinned := s.locked[src.Key]
	pinned = pinned && e.Source == src.Repo && e.Ref == src.Ref && !s.all && !s.update[src.Key]
	if pinned {
		if h, err := hashDir(dest); err == nil && h == e.Hash {
			s.done[src.Key] = e
			return e, nil
		}
	}
	if err := os.RemoveAll(dest); err != nil {
		return module.LockEntry{}, err
	}
	got := module.LockEntry{Source: src.Repo, Ref: src.Ref}
	var err error
	if src.OCI {
		ref := src.Ref
		if pinned {
			ref = e.Digest
		}
		got.Digest, err = pull(src, ref, s.plainHTTP, dest)
	} else {
		got.Commit, err = s.clone(src, e.Commit, pinned, dest)
	}
	if err != nil {
		return module.LockEntry{}, err
	}
	if got.Hash, err = hashDir(dest); err != nil {
		return module.LockEntry{}, err
	}
	if pinned && got.Hash != e.Hash {
		return module.LockEntry{}, fmt.Errorf("%s at %s%s does not hold what %s pinned; run yak sync --update to accept it",
			src.Key, got.Commit, got.Digest, module.LockFile)
	}
	s.done[src.Key] = got
	fmt.Fprintf(s.log, "vendored %s at %s (%s)\n", src.Key, refName(src.Ref), short(got.Commit+got.Digest))
	return got, nil
}

// clone checks src out at the commit pinned when it is, or else at its ref,
// copies it to dest, and answers the commit.
func (s *syncer) clone(src module.Source, commit string, pinned bool, dest string) (string, error) {
	rev := src.Ref
	if pinned {
		rev = commit
	}
	clone := filepath.Join(s.tmp, filepath.FromSlash(src.Key))
	commit, err := checkout(src, rev, clone)
	if err != nil {
		return "", err
	}
	if err := copyTree(clone, dest); err != nil {
		return "", fmt.Errorf("vendoring %s: %w", src.Key, err)
	}
	return commit, nil
}

// short abbreviates a commit or a digest.
func short(id string) string {
	_, id, _ = strings.Cut(id, ":")
	return id[:min(12, len(id))]
}

func refName(ref string) string {
	if ref == "" {
		return "the default branch"
	}
	return ref
}

// remoteDependencies lists the remote dependencies of pkg, by name.
func remoteDependencies(pkg *module.Package) ([]dependency, error) {
	names := make([]string, 0, len(pkg.Dependencies))
	for name, target := range pkg.Dependencies {
		if module.IsRemote(target) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	deps := make([]dependency, 0, len(names))
	for _, name := range names {
		target := pkg.Dependencies[name]
		src, err := module.ParseSource(target)
		if err != nil {
			return nil, fmt.Errorf("%s: dependency %q: %w", pkg.Path, name, err)
		}
		deps = append(deps, dependency{target: target, src: src, from: pkg.Path})
	}
	return deps, nil
}

// readPackage reads the yak.pkg in dir, and answers nil when there is none.
func readPackage(dir string) (*module.Package, error) {
	path := filepath.Join(dir, module.PkgFile)
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return module.ParsePackage(path, src)
}

// dependencyName checks that path, which source names, is a package or a
// module, and answers the name it is added under: name when one is given,
// and otherwise the name of the package or of the module's file.
func dependencyName(path, source, name string) (string, error) {
	derived := ""
	if base := filepath.Base(path); strings.HasSuffix(base, module.Ext) || strings.HasSuffix(base, module.ExtLibrary) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("%s does not exist", source)
		}
		derived = strings.TrimSuffix(strings.TrimSuffix(base, module.ExtLibrary), module.Ext)
	} else {
		pkg, err := readPackage(path)
		if err != nil {
			return "", err
		}
		if pkg == nil {
			return "", fmt.Errorf("%s is not a package: it holds no %s", source, module.PkgFile)
		}
		derived = pkg.Name
	}
	if name != "" {
		return name, nil
	}
	if !module.IsName(derived) {
		return "", fmt.Errorf("%q cannot name a dependency; give it a name with --name", derived)
	}
	return derived, nil
}

// addDependency answers pkg, and the yak.pkg that describes it, with name
// added to its dependencies.
func addDependency(pkg *module.Package, name, target string) (*module.Package, []byte, error) {
	if _, ok := pkg.Dependencies[name]; ok {
		return nil, nil, fmt.Errorf("%s already has a dependency %q; remove it first, or add this one with --name", pkg.Path, name)
	}
	return editDependencies(pkg, func(deps *yaml.Node) {
		deps.Style = 0
		deps.Content = append(deps.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: target})
	})
}

// editDependencies answers the yak.pkg of pkg as it reads after edit has
// changed its dependencies, keeping the rest of it as it was written,
// comments included, and the package it describes. The caller writes it.
func editDependencies(pkg *module.Package, edit func(deps *yaml.Node)) (*module.Package, []byte, error) {
	src, err := os.ReadFile(pkg.Path)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", pkg.Path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s does not hold a mapping", pkg.Path)
	}
	top := doc.Content[0]
	var deps *yaml.Node
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "dependencies" {
			deps = top.Content[i+1]
		}
	}
	if deps == nil {
		deps = &yaml.Node{}
		top.Content = append(top.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "dependencies"}, deps)
	}
	if deps.Kind != yaml.MappingNode {
		*deps = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	edit(deps)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, nil, err
	}
	updated, err := module.ParsePackage(pkg.Path, buf.Bytes())
	if err != nil {
		return nil, nil, err
	}
	return updated, buf.Bytes(), nil
}
