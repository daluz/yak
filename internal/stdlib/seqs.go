package stdlib

import (
	"sort"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// valuesParam is the single parameter of the built-ins that take a sequence.
var valuesParam = params("values")

// The seqs module holds the operations on sequences that a comprehension
// cannot spell. It goes by three names, which import the same module.
func init() {
	keyed := append(params("values"), optional("key", &ast.Null{}))
	predicated := append(params("values"), optional("predicate", &ast.Null{}))
	seqs := []*eval.Builtin{
		{Name: "sort", Params: keyed, Fn: sortSeq},
		{Name: "reverse", Params: valuesParam, Fn: reverse},
		{Name: "unique", Params: valuesParam, Fn: unique},
		{Name: "flatten", Params: valuesParam, Fn: flatten},
		{Name: "take", Params: params("values", "count"), Fn: take},
		{Name: "drop", Params: params("values", "count"), Fn: drop},
		{Name: "range", Params: append(params("start"),
			optional("stop", &ast.Null{}), optional("step", &ast.Int{Value: 1})), Fn: rangeSeq},
		{Name: "sum", Params: valuesParam, Fn: sum},
		{Name: "min", Params: keyed, Fn: extreme("min", -1)},
		{Name: "max", Params: keyed, Fn: extreme("max", 1)},
		{Name: "any", Params: predicated, Fn: quantifier("any", true)},
		{Name: "all", Params: predicated, Fn: quantifier("all", false)},
	}
	for _, path := range []string{"std.seqs", "std.sequences", "std.lists"} {
		module.Register(module.BuiltinModule{Path: path, Builtins: seqs})
	}
}

// seqArg reads an argument that has to be a sequence, along with the values
// of its items.
func seqArg(name string, arg *eval.Thunk) (*eval.Array, []eval.Value, error) {
	v, err := arg.Value()
	if err != nil {
		return nil, nil, err
	}
	a, ok := v.(*eval.Array)
	if !ok {
		return nil, nil, eval.Errorf(arg.Pos(), "%q needs a sequence, found %s", name, v.TypeName())
	}
	values := make([]eval.Value, a.Len())
	for i, item := range a.Items() {
		if values[i], err = item.Value.Value(); err != nil {
			return nil, nil, err
		}
	}
	return a, values, nil
}

// keysOf answers what the key function at arg answers for each of values, or
// the values themselves when the key is null.
func keysOf(values []eval.Value, arg *eval.Thunk) ([]eval.Value, error) {
	key, err := arg.Value()
	if err != nil {
		return nil, err
	}
	if _, none := key.(eval.Null); none {
		return values, nil
	}
	keys := make([]eval.Value, len(values))
	for i, v := range values {
		if keys[i], err = eval.Call(key, arg.Pos(), v); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func sortSeq(args []*eval.Thunk) (eval.Value, error) {
	a, values, err := seqArg("sort", args[0])
	if err != nil {
		return nil, err
	}
	keys, err := keysOf(values, args[1])
	if err != nil {
		return nil, err
	}
	return sortedBy("sort", args[0], a, keys)
}

// sortedBy answers the items of a in the order of keys, keeping items whose
// keys are equal in the order they had. Only numbers and strings have an
// order, and they cannot be mixed, as for "<".
func sortedBy(name string, arg *eval.Thunk, a *eval.Array, keys []eval.Value) (eval.Value, error) {
	order := make([]int, len(keys))
	for i := range order {
		order[i] = i
	}
	var failed error
	sort.SliceStable(order, func(i, j int) bool {
		c, err := eval.Compare(keys[order[i]], keys[order[j]])
		if err != nil && failed == nil {
			failed = err
		}
		return c < 0
	})
	if failed != nil {
		return nil, eval.Errorf(arg.Pos(), "%q cannot order its items: %s", name, failed.Error())
	}
	items := make([]*eval.Elem, len(order))
	for i, at := range order {
		items[i] = a.Items()[at]
	}
	return eval.NewArray(items), nil
}

func reverse(args []*eval.Thunk) (eval.Value, error) {
	a, _, err := seqArg("reverse", args[0])
	if err != nil {
		return nil, err
	}
	items := make([]*eval.Elem, a.Len())
	for i, item := range a.Items() {
		items[len(items)-1-i] = item
	}
	return eval.NewArray(items), nil
}

// unique keeps the first of the items that are equal to each other. Items are
// compared as "==" compares them, so collections cannot be.
func unique(args []*eval.Thunk) (eval.Value, error) {
	a, values, err := seqArg("unique", args[0])
	if err != nil {
		return nil, err
	}
	var items []*eval.Elem
	var kept []eval.Value
	for i, v := range values {
		seen, err := indexOf(kept, v)
		if err != nil {
			return nil, eval.Errorf(args[0].Pos(), "%q cannot compare item %d: %s", "unique", i, err.Error())
		}
		if seen < 0 {
			items = append(items, a.Items()[i])
			kept = append(kept, v)
		}
	}
	return eval.NewArray(items), nil
}

func indexOf(values []eval.Value, v eval.Value) (int, error) {
	for i, have := range values {
		same, err := eval.Equal(have, v)
		if err != nil {
			return -1, err
		}
		if same {
			return i, nil
		}
	}
	return -1, nil
}

// flatten joins a sequence of sequences into one. It goes one level deep,
// and every item has to be a sequence.
func flatten(args []*eval.Thunk) (eval.Value, error) {
	_, values, err := seqArg("flatten", args[0])
	if err != nil {
		return nil, err
	}
	var items []*eval.Elem
	for i, v := range values {
		inner, ok := v.(*eval.Array)
		if !ok {
			return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence of sequences, found %s at index %d",
				"flatten", v.TypeName(), i)
		}
		items = append(items, inner.Items()...)
	}
	return eval.NewArray(items), nil
}

// take keeps at most count items from the start of a sequence.
func take(args []*eval.Thunk) (eval.Value, error) {
	a, n, err := seqAndCount("take", args)
	if err != nil {
		return nil, err
	}
	return eval.NewArray(a.Items()[:min(n, a.Len())]), nil
}

// drop leaves out at most count items from the start of a sequence.
func drop(args []*eval.Thunk) (eval.Value, error) {
	a, n, err := seqAndCount("drop", args)
	if err != nil {
		return nil, err
	}
	return eval.NewArray(a.Items()[min(n, a.Len()):]), nil
}

func seqAndCount(name string, args []*eval.Thunk) (*eval.Array, int, error) {
	a, _, err := seqArg(name, args[0])
	if err != nil {
		return nil, 0, err
	}
	n, err := countArg(name, args[1])
	if err != nil {
		return nil, 0, err
	}
	return a, n, nil
}

// rangeSeq counts as Python's range does: from start up to stop by step,
// leaving stop out. With stop left out the one argument is the stop and the
// count starts at 0, so range(3) is [0, 1, 2]. A negative step counts down,
// and a stop the step cannot reach gives an empty sequence.
func rangeSeq(args []*eval.Thunk) (eval.Value, error) {
	start, err := intArg("range", args[0])
	if err != nil {
		return nil, err
	}
	var stop eval.Int
	if v, err := args[1].Value(); err != nil {
		return nil, err
	} else if _, none := v.(eval.Null); none {
		start, stop = 0, start
	} else if stop, err = intArg("range", args[1]); err != nil {
		return nil, err
	}
	step, err := intArg("range", args[2])
	if err != nil {
		return nil, err
	}
	if step == 0 {
		return nil, eval.Errorf(args[2].Pos(), "%q needs a step other than 0", "range")
	}
	var items []*eval.Elem
	for i := start; (step > 0 && i < stop) || (step < 0 && i > stop); i += step {
		items = append(items, eval.Item(i))
	}
	return eval.NewArray(items), nil
}

// sum adds numbers up as "+" would: integers stay integers until a float
// joins them, and an empty sequence sums to 0.
func sum(args []*eval.Thunk) (eval.Value, error) {
	_, values, err := seqArg("sum", args[0])
	if err != nil {
		return nil, err
	}
	var total eval.Value = eval.Int(0)
	for i, v := range values {
		switch t := v.(type) {
		case eval.Int:
			if n, ok := total.(eval.Int); ok {
				total = n + t
			} else {
				total = total.(eval.Float) + eval.Float(t)
			}
		case eval.Float:
			if n, ok := total.(eval.Int); ok {
				total = eval.Float(n) + t
			} else {
				total = total.(eval.Float) + t
			}
		default:
			return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence of numbers, found %s at index %d",
				"sum", v.TypeName(), i)
		}
	}
	return total, nil
}

// extreme makes min or max, which answer the item that sorts first or last,
// by its key when there is one. want is the sign of a comparison that makes
// a new item the answer, so the first of several equal items is the one.
func extreme(name string, want int) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		_, values, err := seqArg(name, args[0])
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence with at least one item", name)
		}
		keys, err := keysOf(values, args[1])
		if err != nil {
			return nil, err
		}
		best := 0
		for i := 1; i < len(keys); i++ {
			c, err := eval.Compare(keys[i], keys[best])
			if err != nil {
				return nil, eval.Errorf(args[0].Pos(), "%q cannot order its items: %s", name, err.Error())
			}
			if c*want > 0 {
				best = i
			}
		}
		return values[best], nil
	}
}

// quantifier makes any or all, which ask the predicate about each item, or
// read each item as the answer when there is no predicate. Both stop at the
// first item that decides the answer, which is stop for any and the opposite
// of it for all, so the items after it are never handed to the predicate.
func quantifier(name string, stop bool) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		_, values, err := seqArg(name, args[0])
		if err != nil {
			return nil, err
		}
		pred, err := args[1].Value()
		if err != nil {
			return nil, err
		}
		_, none := pred.(eval.Null)
		for i, v := range values {
			r := v
			if !none {
				if r, err = eval.Call(pred, args[1].Pos(), v); err != nil {
					return nil, err
				}
			}
			b, ok := r.(eval.Bool)
			if !ok && none {
				return nil, eval.Errorf(args[0].Pos(), "%q needs a sequence of booleans, found %s at index %d",
					name, r.TypeName(), i)
			}
			if !ok {
				return nil, eval.Errorf(args[1].Pos(), "%q needs a predicate that answers a boolean, found %s at index %d",
					name, r.TypeName(), i)
			}
			if bool(b) == stop {
				return eval.Bool(stop), nil
			}
		}
		return eval.Bool(!stop), nil
	}
}

func intArg(name string, arg *eval.Thunk) (eval.Int, error) {
	v, err := arg.Value()
	if err != nil {
		return 0, err
	}
	n, ok := v.(eval.Int)
	if !ok {
		return 0, eval.Errorf(arg.Pos(), "%q needs an integer, found %s", name, v.TypeName())
	}
	return n, nil
}
