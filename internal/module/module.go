// Package module finds and loads the module an "import" names: a built-in
// module of the standard library, or a file module read from a template file.
//
// A file module's path resolves relative to the directory of the importing
// file, unless the nearest yak.mod above that directory maps the path, or a
// prefix of it, onto somewhere else.
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
	// names a file, and a yak.mod cannot map it.
	Std = "std"
	// ModFile is the name of the file that maps module paths.
	ModFile = "yak.mod"
	// Ext is the extension of a module's file.
	Ext = ".yak"
)

// Resolve returns the file that path names when it is imported by the file
// at from.
func Resolve(from string, path []string) (string, error) {
	dir, segs := filepath.Dir(from), path
	mod, err := find(dir)
	if err != nil {
		return "", err
	}
	if mod != nil {
		target, rest, err := mod.resolve(path)
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

// lookup finds the module that segs names under dir: the directories of
// every segment but the last, and the file that the last one names. name is
// the whole path as the import wrote it.
func lookup(dir string, segs []string, name string) (string, error) {
	base := filepath.Join(append([]string{dir}, segs...)...)
	file := base + Ext
	if _, err := os.Stat(file); err == nil {
		return file, nil
	}
	if info, err := os.Stat(base); err == nil && info.IsDir() {
		return "", fmt.Errorf("%q is a package; import one of the modules in it", name)
	}
	return "", fmt.Errorf("no module %q: %s does not exist", name, file)
}

// modFile is a parsed yak.mod.
type modFile struct {
	path string
	// modules maps a dotted module path onto the location it was written as.
	modules map[string]string
}

// find reads the nearest yak.mod in dir or above it, and answers nil when
// there is none.
func find(dir string) (*modFile, error) {
	for {
		path := filepath.Join(dir, ModFile)
		src, err := os.ReadFile(path)
		if err == nil {
			return parse(path, src)
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

func parse(path string, src []byte) (*modFile, error) {
	var doc struct {
		Modules map[string]string `yaml:"modules"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for key, target := range doc.Modules {
		segs := strings.Split(key, ".")
		for _, s := range segs {
			if !isSegment(s) {
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
	return &modFile{path: path, modules: doc.Modules}, nil
}

// isSegment reports whether s can be written as one segment of an import
// path, which is an identifier.
func isSegment(s string) bool {
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

// resolve applies the longest entry whose key is a prefix of path. It answers
// the location the entry maps to and the segments of path left below it, or
// an empty location when no entry covers the path. A location that is a
// module has nothing left below it; one that is a package always does.
func (m *modFile) resolve(path []string) (string, []string, error) {
	var key []string
	var target string
	for k, t := range m.modules {
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
	if strings.Contains(target, "://") {
		return "", nil, fmt.Errorf("%s maps %q to %s; remote modules are not implemented yet", m.path, name, target)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(m.path), target)
	}
	rest := path[len(key):]
	if strings.HasSuffix(target, Ext) {
		if len(rest) > 0 {
			return "", nil, fmt.Errorf("%s maps %q to the module %s, so %q is not inside a package",
				m.path, name, target, strings.Join(path, "."))
		}
		if _, err := os.Stat(target); err != nil {
			return "", nil, fmt.Errorf("%s maps %q to %s, which does not exist", m.path, name, target)
		}
		return target, nil, nil
	}
	if len(rest) == 0 {
		return "", nil, fmt.Errorf("%s maps %q to the package %s; import one of the modules in it",
			m.path, name, target)
	}
	return target, rest, nil
}
