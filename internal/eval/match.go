package eval

import (
	"fmt"

	"github.com/daluz/yak/internal/ast"
)

// evalMatch answers the value of the first case whose type the value
// matches, read with the names the case binds, or of the "else" when none
// does. A match without an "else" that no case fits is null.
func evalMatch(node *ast.Match, env *Env) (Value, error) {
	v, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	for _, c := range node.Cases {
		r, ok, err := matchCase(c, v, env)
		if err != nil {
			return nil, err
		}
		if ok {
			binds := map[string]*Thunk{}
			if c.Name != "" {
				binds[c.Name] = Done(r)
			}
			caseBinds(c.Type, r, binds)
			return evalNode(c.Value, &Env{doc: env.doc, self: env.self, scope: &scope{outer: env.scope, binds: binds}})
		}
	}
	if node.Else != nil {
		return evalNode(node.Else, env)
	}
	return Null{}, nil
}

// caseSchema is the schema a case checks a value against. It is open, so a
// mapping type the case writes takes keys it does not declare, while a
// schema it names keeps its own rules.
func caseSchema(c *ast.Case, env *Env) *schema {
	return &schema{decl: &ast.Schema{Base: c.Base, Open: true, Type: c.Type}, env: env}
}

// matchCase checks v against the type of c, in full and even when the run
// skips checks, since the answer decides which case is taken. It answers v
// with the defaults the type fills in, and whether it matched.
func matchCase(c *ast.Case, v Value, env *Env) (Value, bool, error) {
	session := env.doc.session
	session.choosing++
	defer func() { session.choosing-- }()
	at := site{of: fmt.Sprintf("the case at %d:%d", c.Pos().Line, c.Pos().Col), pos: c.Pos(), pending: &[]*Thunk{}}
	r, err := caseSchema(c, env).settle(v, c.Type, at, false)
	if err != nil {
		if e, ok := err.(*Error); ok && e.misfit != 0 {
			return nil, false, nil
		}
		return nil, false, err
	}
	return r, true, nil
}

// caseBinds adds to binds the items of v that the type t of a case binds by
// name. v has matched t, so each item it reads is already checked.
func caseBinds(t ast.Node, v Value, binds map[string]*Thunk) {
	switch t := t.(type) {
	case *ast.TypeCheck:
		caseBinds(t.Type, v, binds)
	case *ast.TypeTuple:
		arr := v.(*Array)
		for i, item := range t.Items {
			e := arr.Items()[i]
			if b, ok := item.(*ast.TypeBind); ok {
				binds[b.Name] = e.Value
				item = b.Type
			}
			if iv, err := e.Value.Value(); err == nil {
				caseBinds(item, iv, binds)
			}
		}
	}
}
