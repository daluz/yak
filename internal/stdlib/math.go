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
		builtin("abs", valueParam, abs),
		builtin("floor", valueParam, rounding(math.Floor)),
		builtin("ceil", valueParam, rounding(math.Ceil)),
		builtin("round", append(params("value"), optional("digits", &ast.Null{})), round),
	}})
}

// round rounds to an integer, or a float to digits places after the point
// when digits is given, answering a float as Python does.
func round(c call) (eval.Value, error) {
	d, err := c.value(1)
	if err != nil {
		return nil, err
	}
	if _, none := d.(eval.Null); none {
		return rounding(math.Round)(c)
	}
	digits, err := c.count(1)
	if err != nil {
		return nil, err
	}
	v, err := c.number(0)
	if err != nil {
		return nil, err
	}
	if f, ok := v.(eval.Float); ok {
		scale := math.Pow(10, float64(digits))
		return eval.Float(math.Round(float64(f)*scale) / scale), nil
	}
	return v, nil
}

func abs(c call) (eval.Value, error) {
	v, err := c.number(0)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.Int:
		if t == math.MinInt64 {
			return nil, c.errorf(0, "of %d is too large for an integer", int64(t))
		}
		return eval.Int(max(t, -t)), nil
	default:
		return eval.Float(math.Abs(float64(t.(eval.Float)))), nil
	}
}

// rounding makes a built-in that rounds a float to an integer with fn. round
// takes halves away from zero.
func rounding(fn func(float64) float64) func(call) (eval.Value, error) {
	return func(c call) (eval.Value, error) {
		v, err := c.number(0)
		if err != nil {
			return nil, err
		}
		f, ok := v.(eval.Float)
		if !ok {
			return v, nil
		}
		r := fn(float64(f))
		if math.IsNaN(r) || r < math.MinInt64 || r >= math.MaxInt64 {
			return nil, c.errorf(0, "of %v does not fit in an integer", float64(f))
		}
		return eval.Int(r), nil
	}
}
