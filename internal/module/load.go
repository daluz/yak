package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/parser"
)

// A Module is what an import names: a *BuiltinModule written in Go, or a
// *FileModule read from a template file.
type Module interface {
	// Name is the dotted path the module is imported by.
	Name() string
}

// BuiltinModule is a module of the standard library written in Go. The root
// module has no path, and its built-ins are bare names; any other module is
// reached by importing its path.
type BuiltinModule struct {
	Path     string
	Builtins []*eval.Builtin
}

func (m *BuiltinModule) Name() string { return m.Path }

// FileModule is a module written in yak, which holds exactly one document.
type FileModule struct {
	Path string
	File string
}

func (m *FileModule) Name() string { return m.Path }

// builtin holds every registered BuiltinModule, keyed by its path, the root
// module under "".
var builtin = map[string]*BuiltinModule{}

// Register adds a BuiltinModule to the standard library. It is meant to be
// called from an init function, and panics on a module that cannot be
// registered. The root module may be registered in parts.
func Register(m BuiltinModule) {
	if m.Path == "" {
		root := builtin[""]
		if root == nil {
			root = &BuiltinModule{}
			builtin[""] = root
		}
		for _, b := range m.Builtins {
			for _, have := range root.Builtins {
				if have.Name == b.Name {
					panic(fmt.Sprintf("module: built-in %q registered twice", b.Name))
				}
			}
			root.Builtins = append(root.Builtins, b)
		}
		return
	}
	if strings.Split(m.Path, ".")[0] != Std {
		panic(fmt.Sprintf("module: module %q is not under %q", m.Path, Std))
	}
	if _, ok := builtin[m.Path]; ok {
		panic(fmt.Sprintf("module: module %q registered twice", m.Path))
	}
	builtin[m.Path] = &m
}

// Find returns the module that path names when it is imported by the file at
// from.
func Find(from string, path []string) (Module, error) {
	name := strings.Join(path, ".")
	if path[0] == Std {
		if m, ok := builtin[name]; ok {
			return m, nil
		}
		return nil, fmt.Errorf("the standard library has no module %q", name)
	}
	file, err := Resolve(from, path)
	if err != nil {
		return nil, err
	}
	return &FileModule{Path: name, File: file}, nil
}

// Loader loads the modules of one document, reading each file once however
// many times it is imported. It implements eval.Modules.
type Loader struct {
	// loaded holds the root of every file module read so far, keyed by the
	// absolute path of its file.
	loaded map[string]*eval.Thunk
}

func NewLoader() *Loader {
	return &Loader{loaded: map[string]*eval.Thunk{}}
}

// Root answers the built-ins of the root module.
func (l *Loader) Root() []*eval.Builtin {
	if root := builtin[""]; root != nil {
		return root.Builtins
	}
	return nil
}

// Import answers the root of the module node names. A built-in module is a
// mapping of its built-ins; a file module is the value of its one document,
// evaluated in s.
func (l *Loader) Import(s *eval.Session, node *ast.Import) (*eval.Thunk, error) {
	m, err := Find(node.Pos().File, node.Path)
	if err != nil {
		return nil, eval.Errorf(node.Pos(), "%s", err.Error())
	}
	switch m := m.(type) {
	case *BuiltinModule:
		o := eval.NewObject()
		for _, b := range m.Builtins {
			o.Set(b.Name, false, eval.Done(b))
		}
		return eval.Done(o), nil
	case *FileModule:
		return l.load(s, node, m)
	default:
		panic(fmt.Sprintf("module: unknown module %T", m))
	}
}

func (l *Loader) load(s *eval.Session, node *ast.Import, m *FileModule) (*eval.Thunk, error) {
	key, err := filepath.Abs(m.File)
	if err != nil {
		return nil, eval.Errorf(node.Pos(), "%s", err.Error())
	}
	if root, ok := l.loaded[key]; ok {
		return root, nil
	}
	src, err := os.ReadFile(m.File)
	if err != nil {
		return nil, eval.Errorf(node.Pos(), "reading module %q: %s", m.Name(), err.Error())
	}
	stream, err := parser.Parse(m.File, src)
	if err != nil {
		return nil, err
	}
	if n := len(stream.Docs); n != 1 {
		return nil, eval.Errorf(node.Pos(), "module %q holds %d documents; a module holds exactly one",
			m.Name(), n)
	}
	root := s.Document(stream.Docs[0], m.File)
	l.loaded[key] = root
	return root, nil
}
