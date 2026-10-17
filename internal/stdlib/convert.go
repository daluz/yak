package stdlib

import (
	"math"
	"strconv"
	"strings"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The conversions move a value between the scalar types. They are strict: a
// value that would lose something, or a string that does not spell a value
// of the type exactly, is an error rather than a guess.
func init() {
	module.Register(module.BuiltinModule{Builtins: []*eval.Builtin{
		builtin("int", valueParam, toInt),
		builtin("float", valueParam, toFloat),
		builtin("bool", valueParam, toBool),
		builtin("str", valueParam, toStr),
		builtin("type", valueParam, typeOf),
	}})
}

// toInt keeps an integer, takes a float only when it has no fractional part,
// and parses a string that holds an integer literal.
func toInt(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.Int:
		return t, nil
	case eval.Float:
		f := float64(t)
		if f != math.Trunc(f) {
			return nil, c.errorf(0, "cannot convert %v to an integer without losing its fraction; round it with std.math", f)
		}
		if f < math.MinInt64 || f >= math.MaxInt64 {
			return nil, c.errorf(0, "of %v does not fit in an integer", f)
		}
		return eval.Int(f), nil
	case eval.String:
		n, err := strconv.ParseInt(string(t), 10, 64)
		if err != nil {
			return nil, c.errorf(0, "cannot convert %q to an integer", string(t))
		}
		return eval.Int(n), nil
	default:
		return nil, c.errorf(0, "needs a number or a string, found %s", v.TypeName())
	}
}

// toFloat widens an integer and parses a string that holds a finite decimal
// number; the hexadecimal floats strconv also reads are refused.
func toFloat(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.Int:
		return eval.Float(t), nil
	case eval.Float:
		return t, nil
	case eval.String:
		f, err := strconv.ParseFloat(string(t), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || strings.ContainsAny(string(t), "xX") {
			return nil, c.errorf(0, "cannot convert %q to a float", string(t))
		}
		return eval.Float(f), nil
	default:
		return nil, c.errorf(0, "needs a number or a string, found %s", v.TypeName())
	}
}

// toBool keeps a boolean and parses only "true" and "false", leaving out the
// yes, no, on and off of YAML 1.1.
func toBool(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.Bool:
		return t, nil
	case eval.String:
		switch t {
		case "true":
			return eval.Bool(true), nil
		case "false":
			return eval.Bool(false), nil
		}
		return nil, c.errorf(0, "needs \"true\" or \"false\", found %q", string(t))
	default:
		return nil, c.errorf(0, "needs a boolean or a string, found %s", v.TypeName())
	}
}

// toStr writes a scalar as interpolating it would. null is refused rather
// than written as "null", and a collection is rendered with render instead.
func toStr(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case eval.String:
		return t, nil
	case eval.Bool:
		return eval.String(strconv.FormatBool(bool(t))), nil
	case eval.Int:
		return eval.String(strconv.FormatInt(int64(t), 10)), nil
	case eval.Float:
		return eval.String(strconv.FormatFloat(float64(t), 'g', -1, 64)), nil
	default:
		return nil, c.errorf(0, "needs a string, number or boolean, found %s", v.TypeName())
	}
}

func typeOf(c call) (eval.Value, error) {
	v, err := c.value(0)
	if err != nil {
		return nil, err
	}
	return eval.String(v.TypeName()), nil
}
