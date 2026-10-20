package stdlib

import (
	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The operations on mappings are in the root module. Hidden entries count, as
// they do for size, and entries keep the order they were written in.
func init() {
	mapParam := params("mapping")
	module.Register(module.BuiltinModule{Builtins: []*eval.Builtin{
		builtin("keys", mapParam, keys),
		builtin("values", mapParam, values),
		builtin("items", mapParam, items),
		builtin("pick", params("mapping", "keys"), selectKeys(true)),
		builtin("omit", params("mapping", "keys"), selectKeys(false)),
		builtin("visible", mapParam, visible),
		builtin("merge", append(params("x", "y"),
			optional("deep", &ast.Bool{Value: true}), optional("append", &ast.Bool{})), merge),
	}})
}

// merge is "<<" with the choices it makes named: deep merges mappings found
// on both sides at every depth, and append joins sequences found on both.
func merge(c call) (eval.Value, error) {
	x, err := c.mapping(0)
	if err != nil {
		return nil, err
	}
	y, err := c.mapping(1)
	if err != nil {
		return nil, err
	}
	deep, err := c.boolean(2)
	if err != nil {
		return nil, err
	}
	appendSeqs, err := c.boolean(3)
	if err != nil {
		return nil, err
	}
	return eval.MergeWith(x, y, deep, appendSeqs)
}

func keys(c call) (eval.Value, error) {
	o, err := c.mapping(0)
	if err != nil {
		return nil, err
	}
	items := make([]*eval.Elem, o.Len())
	for i, name := range o.Names() {
		items[i] = eval.Item(eval.String(name))
	}
	return eval.NewArray(items), nil
}

func values(c call) (eval.Value, error) {
	o, err := c.mapping(0)
	if err != nil {
		return nil, err
	}
	items := make([]*eval.Elem, o.Len())
	for i, f := range o.Fields() {
		items[i] = &eval.Elem{Value: f.Value}
	}
	return eval.NewArray(items), nil
}

// items answers the entries as [key, value] pairs, which is how a
// comprehension walks a mapping. The values stay lazy, as they do for values.
func items(c call) (eval.Value, error) {
	o, err := c.mapping(0)
	if err != nil {
		return nil, err
	}
	pairs := make([]*eval.Elem, o.Len())
	for i, f := range o.Fields() {
		pair := []*eval.Elem{eval.Item(eval.String(f.Name)), {Value: f.Value}}
		pairs[i] = eval.Item(eval.NewArray(pair))
	}
	return eval.NewArray(pairs), nil
}

// selectKeys makes pick, which keeps the entries named in keys, and omit,
// which keeps the others. A key the mapping does not hold is no error, and
// the entries keep the mapping's order rather than that of keys.
func selectKeys(keep bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		o, err := c.mapping(0)
		if err != nil {
			return nil, err
		}
		names, err := c.strs(1)
		if err != nil {
			return nil, err
		}
		named := make(map[string]bool, len(names))
		for _, name := range names {
			named[name] = true
		}
		out := eval.NewObject()
		for _, f := range o.Fields() {
			if named[f.Name] == keep {
				out.Add(f)
			}
		}
		return out, nil
	}
}

// visible answers the mapping without its "::" entries. A ":?" entry stays,
// since whether it is written out depends on a value that is still lazy.
func visible(c call) (eval.Value, error) {
	o, err := c.mapping(0)
	if err != nil {
		return nil, err
	}
	out := eval.NewObject()
	for _, f := range o.Fields() {
		if !f.Hidden {
			out.Add(f)
		}
	}
	return out, nil
}
