package stdlib

import (
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The maps module holds the operations on mappings. Hidden entries count, as
// they do for size, and entries keep the order they were written in. It goes
// by two names, which import the same module.
func init() {
	mapParam := params("mapping")
	maps := []*eval.Builtin{
		{Name: "keys", Params: mapParam, Fn: keys},
		{Name: "values", Params: mapParam, Fn: values},
		{Name: "pick", Params: params("mapping", "keys"), Fn: selectKeys("pick", true)},
		{Name: "omit", Params: params("mapping", "keys"), Fn: selectKeys("omit", false)},
	}
	for _, path := range []string{"std.maps", "std.objects"} {
		module.Register(module.BuiltinModule{Path: path, Builtins: maps})
	}
}

func mapArg(name string, arg *eval.Thunk) (*eval.Object, error) {
	v, err := arg.Value()
	if err != nil {
		return nil, err
	}
	o, ok := v.(*eval.Object)
	if !ok {
		return nil, eval.Errorf(arg.Pos(), "%q needs a mapping, found %s", name, v.TypeName())
	}
	return o, nil
}

func keys(args []*eval.Thunk) (eval.Value, error) {
	o, err := mapArg("keys", args[0])
	if err != nil {
		return nil, err
	}
	items := make([]*eval.Elem, o.Len())
	for i, name := range o.Names() {
		items[i] = eval.Item(eval.String(name))
	}
	return eval.NewArray(items), nil
}

func values(args []*eval.Thunk) (eval.Value, error) {
	o, err := mapArg("values", args[0])
	if err != nil {
		return nil, err
	}
	items := make([]*eval.Elem, o.Len())
	for i, f := range o.Fields() {
		items[i] = &eval.Elem{Value: f.Value}
	}
	return eval.NewArray(items), nil
}

// selectKeys makes pick, which keeps the entries named in keys, and omit,
// which keeps the others. A key the mapping does not hold is no error, and
// the entries keep the mapping's order rather than that of keys.
func selectKeys(name string, keep bool) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		o, err := mapArg(name, args[0])
		if err != nil {
			return nil, err
		}
		_, values, err := seqArg(name, args[1])
		if err != nil {
			return nil, err
		}
		named := make(map[string]bool, len(values))
		for i, v := range values {
			s, ok := v.(eval.String)
			if !ok {
				return nil, eval.Errorf(args[1].Pos(), "%q needs a sequence of strings, found %s at index %d",
					name, v.TypeName(), i)
			}
			named[string(s)] = true
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
