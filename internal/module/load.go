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

// BuiltinModule is a module of the standard library written in Go. The root
// module has no path, and its built-ins are bare names; any other module is
// reached by importing its path.
type BuiltinModule struct {
	Path     string
	Builtins []*eval.Builtin
	// Nondeterministic marks a module whose answers can change from one
	// rendering to the next, which strict mode refuses to import.
	Nondeterministic bool
	// StrictBuiltins, when set, replace Builtins in strict mode, for a
	// module that can do more when strict mode is off.
	StrictBuiltins []*eval.Builtin
}

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

// Loader loads the modules of one document, reading each file once however
// many times it is imported. It implements eval.Modules.
type Loader struct {
	// loaded holds the root of every file module read so far, keyed by the
	// absolute path of its file.
	loaded map[string]*eval.Thunk
	// strict admits only the built-in modules that answer the same on every
	// rendering, in their strict form.
	strict bool
}

func NewLoader(strict bool) *Loader {
	return &Loader{loaded: map[string]*eval.Thunk{}, strict: strict}
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
	name := node.Name()
	if node.File != "" {
		if l.strict {
			return nil, eval.Errorf(node.Pos(),
				"module %q is imported by its file path, which strict mode does not allow; render with --strict=false to use it",
				name)
		}
		file, err := ResolveFile(node.Pos().File, node.File)
		if err != nil {
			return nil, eval.Errorf(node.Pos(), "%s", err.Error())
		}
		return l.load(s, node, name, file)
	}
	if node.Path[0] != Std {
		file, err := Resolve(node.Pos().File, node.Path)
		if err != nil {
			return nil, eval.Errorf(node.Pos(), "%s", err.Error())
		}
		return l.load(s, node, name, file)
	}
	m, ok := builtin[name]
	if !ok {
		return nil, eval.Errorf(node.Pos(), "the standard library has no module %q", name)
	}
	if l.strict && m.Nondeterministic {
		return nil, eval.Errorf(node.Pos(),
			"module %q can answer differently on every rendering, which strict mode does not allow; render with --strict=false to use it",
			m.Path)
	}
	builtins := m.Builtins
	if l.strict && m.StrictBuiltins != nil {
		builtins = m.StrictBuiltins
	}
	o := eval.NewObject()
	for _, b := range builtins {
		o.Set(b.Name, false, eval.Done(b))
	}
	return eval.Done(o), nil
}

// load reads the file module named name from file.
func (l *Loader) load(s *eval.Session, node *ast.Import, name, file string) (*eval.Thunk, error) {
	key, err := filepath.Abs(file)
	if err != nil {
		return nil, eval.Errorf(node.Pos(), "%s", err.Error())
	}
	if root, ok := l.loaded[key]; ok {
		return root, nil
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, eval.Errorf(node.Pos(), "reading module %q: %s", name, err.Error())
	}
	stream, err := parser.Parse(file, src)
	if err != nil {
		return nil, err
	}
	if n := len(stream.Docs); n != 1 {
		return nil, eval.Errorf(node.Pos(), "module %q holds %d documents; a module holds exactly one", name, n)
	}
	root := s.Document(stream.Docs[0], file)
	l.loaded[key] = root
	return root, nil
}
