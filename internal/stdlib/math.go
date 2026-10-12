package stdlib

import (
	"math"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The math module rounds numbers. Rounding a float answers an integer, so
// that its result can count things, and an integer is already as round as it
// gets.
func init() {
	module.Register(module.BuiltinModule{Path: "std.math", Builtins: []*eval.Builtin{
		{Name: "abs", Params: valueParam, Fn: abs},
		{Name: "floor", Params: valueParam, Fn: rounding("floor", math.Floor)},
		{Name: "ceil", Params: valueParam, Fn: rounding("ceil", math.Ceil)},
		{Name: "round", Params: append(params("value"), optional("digits", &ast.Null{})), Fn: round},
	}})
}

// round rounds to an integer, or a float to digits places after the point
// when digits is given, answering a float as Python does.
func round(args []*eval.Thunk) (eval.Value, error) {
	d, err := args[1].Value()
	if err != nil {
		return nil, err
	}
	if _, none := d.(eval.Null); none {
		return rounding("round", math.Round)(args[:1])
	}
	digits, err := countArg("round", args[1])
	if err != nil {
		return nil, err
	}
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.Int:
		return t, nil
	case eval.Float:
		scale := math.Pow(10, float64(digits))
		return eval.Float(math.Round(float64(t)*scale) / scale), nil
	default:
		return nil, eval.Errorf(args[0].Pos(), "%q needs a number, found %s", "round", v.TypeName())
	}
}

func abs(args []*eval.Thunk) (eval.Value, error) {
	v, err := args[0].Value()
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.Int:
		if t == math.MinInt64 {
			return nil, eval.Errorf(args[0].Pos(), "%q of %d is too large for an integer", "abs", int64(t))
		}
		return eval.Int(max(t, -t)), nil
	case eval.Float:
		return eval.Float(math.Abs(float64(t))), nil
	default:
		return nil, eval.Errorf(args[0].Pos(), "%q needs a number, found %s", "abs", v.TypeName())
	}
}

// rounding makes a built-in that rounds a float to an integer with fn. round
// takes halves away from zero.
func rounding(name string, fn func(float64) float64) func([]*eval.Thunk) (eval.Value, error) {
	return func(args []*eval.Thunk) (eval.Value, error) {
		v, err := args[0].Value()
		if err != nil {
			return nil, err
		}
		switch t := v.(type) {
		case eval.Int:
			return t, nil
		case eval.Float:
			r := fn(float64(t))
			if math.IsNaN(r) || r < math.MinInt64 || r >= math.MaxInt64 {
				return nil, eval.Errorf(args[0].Pos(), "%q of %v does not fit in an integer", name, float64(t))
			}
			return eval.Int(r), nil
		default:
			return nil, eval.Errorf(args[0].Pos(), "%q needs a number, found %s", name, v.TypeName())
		}
	}
}
