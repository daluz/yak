package stdlib

import (
	"sort"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The operations on sequences that a comprehension cannot spell are in the
// root module.
func init() {
	keyed := append(params("values"), optional("key", &ast.Null{}))
	predicated := append(params("values"), optional("predicate", &ast.Null{}))
	module.Register(module.BuiltinModule{Builtins: []*eval.Builtin{
		builtin("sort", keyed, sortSeq),
		builtin("reverse", valuesParam, reverse),
		builtin("unique", valuesParam, unique),
		builtin("flatten", valuesParam, flatten),
		builtin("slice", append(params("values", "start"), optional("stop", &ast.Null{})), slice),
		builtin("range", append(params("start"),
			optional("stop", &ast.Null{}), optional("step", &ast.Int{Value: 1})), rangeSeq),
		builtin("sum", valuesParam, sum),
		builtin("min", keyed, extreme(-1)),
		builtin("max", keyed, extreme(1)),
		builtin("some", predicated, quantifier(true)),
		builtin("every", predicated, quantifier(false)),
	}})
}

// keysOf answers what the key function at argument i answers for each of
// values, or the values themselves when the key is null.
func keysOf(c call, i int, values []eval.Value) ([]eval.Value, error) {
	key, err := c.value(i)
	if err != nil {
		return nil, err
	}
	if _, none := key.(eval.Null); none {
		return values, nil
	}
	keys := make([]eval.Value, len(values))
	for j, v := range values {
		if keys[j], err = eval.Call(key, c.pos(i), v); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func sortSeq(c call) (eval.Value, error) {
	a, values, err := c.seq(0)
	if err != nil {
		return nil, err
	}
	keys, err := keysOf(c, 1, values)
	if err != nil {
		return nil, err
	}
	return sortedBy(c, a, keys)
}

// sortedBy answers the items of a in the order of keys, keeping items whose
// keys are equal in the order they had. Only numbers and strings have an
// order, and they cannot be mixed, as for "<".
func sortedBy(c call, a *eval.Array, keys []eval.Value) (eval.Value, error) {
	order := make([]int, len(keys))
	for i := range order {
		order[i] = i
	}
	var failed error
	sort.SliceStable(order, func(i, j int) bool {
		cmp, err := eval.Compare(keys[order[i]], keys[order[j]])
		if err != nil && failed == nil {
			failed = err
		}
		return cmp < 0
	})
	if failed != nil {
		return nil, c.errorf(0, "cannot order its items: %s", failed.Error())
	}
	items := make([]*eval.Elem, len(order))
	for i, at := range order {
		items[i] = a.Items()[at]
	}
	return eval.NewArray(items), nil
}

func reverse(c call) (eval.Value, error) {
	a, err := c.array(0)
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
// compared as "==" compares them, so collections and functions cannot be.
func unique(c call) (eval.Value, error) {
	a, values, err := c.seq(0)
	if err != nil {
		return nil, err
	}
	var items []*eval.Elem
	var kept []eval.Value
	for i, v := range values {
		seen, err := contains(kept, v)
		if err != nil {
			return nil, c.errorf(0, "cannot compare item %d: %s", i, err.Error())
		}
		if !seen {
			items = append(items, a.Items()[i])
			kept = append(kept, v)
		}
	}
	return eval.NewArray(items), nil
}

func contains(values []eval.Value, v eval.Value) (bool, error) {
	for _, have := range values {
		same, err := eval.Equal(have, v)
		if err != nil || same {
			return same, err
		}
	}
	return false, nil
}

// flatten joins a sequence of sequences into one. It goes one level deep,
// and every item has to be a sequence.
func flatten(c call) (eval.Value, error) {
	_, values, err := c.seq(0)
	if err != nil {
		return nil, err
	}
	var items []*eval.Elem
	for i, v := range values {
		inner, ok := v.(*eval.Array)
		if !ok {
			return nil, c.errorf(0, "needs a sequence of sequences, found %s at index %d", v.TypeName(), i)
		}
		items = append(items, inner.Items()...)
	}
	return eval.NewArray(items), nil
}

// slice keeps the items from start up to but not including stop, or to the
// end when stop is null. A negative position counts from the end, as an index
// does, and a position past either end stops there, so a stop before the
// start gives an empty sequence.
func slice(c call) (eval.Value, error) {
	a, err := c.array(0)
	if err != nil {
		return nil, err
	}
	start, err := c.integer(1)
	if err != nil {
		return nil, err
	}
	stop, given, err := c.optionalInt(2)
	if err != nil {
		return nil, err
	}
	if !given {
		stop = eval.Int(a.Len())
	}
	lo, hi := position(start, a.Len()), position(stop, a.Len())
	return eval.NewArray(a.Items()[lo:max(lo, hi)]), nil
}

// position places n in a sequence of the given length, counting a negative n
// from the end and stopping at either end.
func position(n eval.Int, length int) int {
	i := int(n)
	if i < 0 {
		i += length
	}
	return min(max(i, 0), length)
}

// rangeSeq counts as Python's range does: from start up to stop by step,
// leaving stop out. With stop left out the one argument is the stop and the
// count starts at 0, so range(3) is [0, 1, 2]. A negative step counts down,
// and a stop the step cannot reach gives an empty sequence.
func rangeSeq(c call) (eval.Value, error) {
	start, err := c.integer(0)
	if err != nil {
		return nil, err
	}
	stop, given, err := c.optionalInt(1)
	if err != nil {
		return nil, err
	}
	if !given {
		start, stop = 0, start
	}
	step, err := c.integer(2)
	if err != nil {
		return nil, err
	}
	if step == 0 {
		return nil, c.errorf(2, "needs a step other than 0")
	}
	var items []*eval.Elem
	for i := start; (step > 0 && i < stop) || (step < 0 && i > stop); i += step {
		items = append(items, eval.Item(i))
		if (step > 0) != (i+step > i) {
			break
		}
	}
	return eval.NewArray(items), nil
}

// sum adds numbers up as "+" would: integers stay integers until a float
// joins them, and an empty sequence sums to 0.
func sum(c call) (eval.Value, error) {
	_, values, err := c.seq(0)
	if err != nil {
		return nil, err
	}
	var total eval.Value = eval.Int(0)
	for i, v := range values {
		switch v.(type) {
		case eval.Int, eval.Float:
		default:
			return nil, c.errorf(0, "needs a sequence of numbers, found %s at index %d", v.TypeName(), i)
		}
		if total, err = eval.Add(total, v); err != nil {
			return nil, c.errorf(0, "cannot add item %d: %s", i, err.Error())
		}
	}
	return total, nil
}

// extreme makes min or max, which answer the item that sorts first or last,
// by its key when there is one. want is the sign of a comparison that makes
// a new item the answer, so the first of several equal items is the one.
func extreme(want int) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		_, values, err := c.seq(0)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, c.errorf(0, "needs a sequence with at least one item")
		}
		keys, err := keysOf(c, 1, values)
		if err != nil {
			return nil, err
		}
		best := 0
		for i := 1; i < len(keys); i++ {
			cmp, err := eval.Compare(keys[i], keys[best])
			if err != nil {
				return nil, c.errorf(0, "cannot order its items: %s", err.Error())
			}
			if cmp*want > 0 {
				best = i
			}
		}
		return values[best], nil
	}
}

// quantifier makes some or every, which ask the predicate about each item, or
// read each item as the answer when there is no predicate. Both stop at the
// first item that decides the answer, which is stop for some and the opposite
// of it for every, so the items after it are never handed to the predicate.
func quantifier(stop bool) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		a, err := c.array(0)
		if err != nil {
			return nil, err
		}
		pred, err := c.value(1)
		if err != nil {
			return nil, err
		}
		_, none := pred.(eval.Null)
		for i, item := range a.Items() {
			v, err := item.Value.Value()
			if err != nil {
				return nil, err
			}
			r := v
			if !none {
				if r, err = eval.Call(pred, c.pos(1), v); err != nil {
					return nil, err
				}
			}
			b, ok := r.(eval.Bool)
			if !ok {
				if none {
					return nil, c.errorf(0, "needs a sequence of booleans, found %s at index %d", r.TypeName(), i)
				}
				return nil, c.errorf(1, "needs a predicate that answers a boolean, found %s at index %d",
					r.TypeName(), i)
			}
			if bool(b) == stop {
				return eval.Bool(stop), nil
			}
		}
		return eval.Bool(!stop), nil
	}
}
