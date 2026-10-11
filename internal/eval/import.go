package eval

import "github.com/daluz/yak/internal/ast"

func evalImport(node *ast.Import, env *Env) (Value, error) {
	s := env.doc.session
	root, err := s.mods.Import(s, node)
	if err != nil {
		return nil, err
	}
	return root.Value()
}
