package eval

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// evalIf leaves the branch that was not taken unevaluated, so a conditional
// may guard an expression that only makes sense when the condition holds.
// A conditional written without an "else" yields null.
func evalIf(node *ast.If, env *Env) (Value, error) {
	cond, err := condition(node.Cond, env, `the condition of an "if"`)
	if err != nil {
		return nil, err
	}
	if cond {
		return evalNode(node.Then, env)
	}
	if node.Else == nil {
		return Null{}, nil
	}
	return evalNode(node.Else, env)
}

func evalUnary(node *ast.Unary, env *Env) (Value, error) {
	if node.Op == token.Dash {
		return evalNegate(node, env)
	}
	x, err := condition(node.X, env, `the operand of "!"`)
	if err != nil {
		return nil, err
	}
	return Bool(!x), nil
}

func evalNegate(node *ast.Unary, env *Env) (Value, error) {
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	switch t := x.(type) {
	case Int:
		return -t, nil
	case Float:
		return -t, nil
	}
	return nil, errorf(node.X.Pos(), `the operand of "-" must be a number, found %s`, x.TypeName())
}

func evalBinary(node *ast.Binary, env *Env) (Value, error) {
	if node.Op == token.And || node.Op == token.Or {
		return evalLogical(node, env)
	}
	x, err := evalNode(node.X, env)
	if err != nil {
		return nil, err
	}
	y, err := evalNode(node.Y, env)
	if err != nil {
		return nil, err
	}
	switch node.Op {
	case token.Eq, token.Ne:
		same, err := equal(x, y)
		if err != nil {
			return nil, errorf(node.OpPos, "%s", err.Error())
		}
		return Bool(same == (node.Op == token.Eq)), nil
	case token.Plus, token.Dash, token.Star, token.Slash, token.Percent:
		v, err := arith(node.Op, x, y)
		if err != nil {
			return nil, errorf(node.OpPos, "%s", err.Error())
		}
		return v, nil
	}
	c, err := compare(x, y)
	if err != nil {
		return nil, errorf(node.OpPos, "%s", err.Error())
	}
	switch node.Op {
	case token.Lt:
		return Bool(c < 0), nil
	case token.Le:
		return Bool(c <= 0), nil
	case token.Gt:
		return Bool(c > 0), nil
	default:
		return Bool(c >= 0), nil
	}
}

// evalLogical short-circuits, so the right hand side is left unevaluated
// whenever the left one already decides the answer.
func evalLogical(node *ast.Binary, env *Env) (Value, error) {
	name := `"&&"`
	if node.Op == token.Or {
		name = `"||"`
	}
	x, err := condition(node.X, env, "the left side of "+name)
	if err != nil {
		return nil, err
	}
	if x == (node.Op == token.Or) {
		return Bool(x), nil
	}
	y, err := condition(node.Y, env, "the right side of "+name)
	if err != nil {
		return nil, err
	}
	return Bool(y), nil
}

// condition evaluates an operand that has to answer a boolean, naming what
// required one when it does not. yak has no truthiness: only a boolean will
// do.
func condition(n ast.Node, env *Env, what string) (bool, error) {
	v, err := evalNode(n, env)
	if err != nil {
		return false, err
	}
	b, ok := v.(Bool)
	if !ok {
		return false, errorf(n.Pos(), "%s must be a boolean, found %s", what, v.TypeName())
	}
	return bool(b), nil
}

// arith applies an arithmetic operator. Every operator wants two numbers;
// "+" also joins two strings, two sequences or two mappings.
//
// Two integers answer an integer, except under "/", which always answers a
// float so that 7 / 2 is 3.5 rather than a silently truncated 3. A float on
// either side makes the answer a float.
func arith(op token.Kind, x, y Value) (Value, error) {
	if op == token.Plus {
		switch a := x.(type) {
		case String:
			if b, ok := y.(String); ok {
				return a + b, nil
			}
		case *Array:
			if b, ok := y.(*Array); ok {
				return concat(a, b), nil
			}
		case *Object:
			if b, ok := y.(*Object); ok {
				return merge(a, b), nil
			}
		}
	}
	a, aok := numeric(x)
	b, bok := numeric(y)
	if !aok || !bok {
		if op == token.Plus {
			return nil, fmt.Errorf("cannot add %s to %s", y.TypeName(), x.TypeName())
		}
		return nil, fmt.Errorf("%s needs two numbers, found %s and %s", op, x.TypeName(), y.TypeName())
	}
	if b == 0 && (op == token.Slash || op == token.Percent) {
		return nil, errors.New("division by zero")
	}
	ai, aInt := x.(Int)
	bi, bInt := y.(Int)
	if aInt && bInt && op != token.Slash {
		switch op {
		case token.Plus:
			return ai + bi, nil
		case token.Dash:
			return ai - bi, nil
		case token.Star:
			return ai * bi, nil
		default:
			return ai % bi, nil
		}
	}
	switch op {
	case token.Plus:
		return Float(a + b), nil
	case token.Dash:
		return Float(a - b), nil
	case token.Star:
		return Float(a * b), nil
	case token.Slash:
		return Float(a / b), nil
	default:
		return Float(math.Mod(a, b)), nil
	}
}

// concat joins two sequences. The items are shared rather than copied, so an
// item still resolves in the sequence it was written in.
func concat(x, y *Array) *Array {
	items := make([]*Elem, 0, x.Len()+y.Len())
	items = append(items, x.items...)
	return NewArray(append(items, y.items...))
}

// merge joins two mappings. A key held by both takes its value from y and
// keeps the position it has in x, which is what makes "+" an override: the
// mapping on the right decides, and the shape of the one on the left stays
// recognisable.
//
// The merge is one level deep. A key whose value is a mapping on both sides
// is replaced outright rather than merged, so what "+" does can be read off
// the two mappings without resolving anything.
func merge(x, y *Object) *Object {
	o := NewObject()
	for _, f := range x.fields {
		o.add(f)
	}
	for _, f := range y.fields {
		o.add(f)
	}
	return o
}

// equal compares two values for "==". Values of different types are never
// equal, except that integers and floats compare as the numbers they are.
func equal(x, y Value) (bool, error) {
	if err := requireScalar(x); err != nil {
		return false, err
	}
	if err := requireScalar(y); err != nil {
		return false, err
	}
	switch a := x.(type) {
	case Null:
		_, ok := y.(Null)
		return ok, nil
	case Bool:
		b, ok := y.(Bool)
		return ok && a == b, nil
	case String:
		b, ok := y.(String)
		return ok && a == b, nil
	}
	c, err := compare(x, y)
	return err == nil && c == 0, nil
}

// compare orders two values, answering a negative number when x sorts first.
// Numbers and strings are the only things with an order.
func compare(x, y Value) (int, error) {
	if a, ok := x.(Int); ok {
		if b, ok := y.(Int); ok {
			switch {
			case a < b:
				return -1, nil
			case a > b:
				return 1, nil
			default:
				return 0, nil
			}
		}
	}
	if a, ok := x.(String); ok {
		if b, ok := y.(String); ok {
			return strings.Compare(string(a), string(b)), nil
		}
	}
	a, aok := numeric(x)
	b, bok := numeric(y)
	if aok && bok {
		switch {
		case a < b:
			return -1, nil
		case a > b:
			return 1, nil
		default:
			return 0, nil
		}
	}
	return 0, fmt.Errorf("cannot compare %s with %s", x.TypeName(), y.TypeName())
}

// requireScalar rejects the values that equality has no useful answer for.
func requireScalar(v Value) error {
	switch v.(type) {
	case *Object, *Array:
		return fmt.Errorf("cannot compare a %s", v.TypeName())
	}
	return nil
}

func numeric(v Value) (float64, bool) {
	switch t := v.(type) {
	case Int:
		return float64(t), true
	case Float:
		return float64(t), true
	}
	return 0, false
}
