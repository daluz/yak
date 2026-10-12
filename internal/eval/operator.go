package eval

import (
	"cmp"
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
	x, err := condition(node.X, env, "the operand of "+node.Op.String())
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
		same, err := Equal(x, y)
		if err != nil {
			return nil, errorAt(node.OpPos, err)
		}
		return Bool(same == (node.Op == token.Eq)), nil
	case token.Plus, token.Dash, token.Star, token.Slash, token.Percent:
		v, err := arith(node.Op, x, y)
		if err != nil {
			return nil, errorAt(node.OpPos, err)
		}
		return v, nil
	case token.Merge:
		a, aok := x.(*Object)
		b, bok := y.(*Object)
		if !aok || !bok {
			return nil, errorf(node.OpPos, "%s needs two mappings, found %s and %s", node.Op, x.TypeName(), y.TypeName())
		}
		v, err := Merge(a, b)
		if err != nil {
			return nil, err
		}
		return v, nil
	case token.In, token.NotIn:
		found, err := member(node.Op, node.OpPos, x, y)
		if err != nil {
			return nil, err
		}
		return Bool(found == (node.Op == token.In)), nil
	}
	c, err := Compare(x, y)
	if err != nil {
		return nil, errorAt(node.OpPos, err)
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
	name := node.Op.String()
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
// "+" also joins two strings or two sequences.
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
			if _, ok := y.(*Object); ok {
				return nil, fmt.Errorf("cannot add two mappings; %q merges them", "<<")
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

// Merge joins two mappings as "<<" does. The one on the right decides: a key
// held by both takes its value from y and keeps the position it has in x, so
// the shape of the mapping on the left stays recognisable.
//
// A key whose value is a mapping on both sides merges rather than being
// replaced, however deep that goes. Anything else y holds replaces what x
// held, so a sequence or a scalar overrides rather than combining.
func Merge(x, y *Object) (*Object, error) {
	o := NewObject()
	for _, f := range x.fields {
		o.Add(f)
	}
	for _, f := range y.fields {
		if prev, held := o.Lookup(f.Name); held {
			value, err := combined(prev, f)
			if err != nil {
				return nil, err
			}
			// The entry keeps everything else the right hand side said
			// about it; only its value is combined.
			copied := *f
			copied.Value = value
			o.Add(&copied)
			continue
		}
		o.Add(f)
	}
	return o, nil
}

// combined answers the value a key takes when a merge finds it on both
// sides: the two mappings merge, and anything else new replaces old.
//
// Telling a mapping from anything else means resolving the value, which is
// why a key held by both sides is evaluated where the merge is written
// rather than where the merged mapping is read.
func combined(old, new *Field) (*Thunk, error) {
	prev, err := old.Value.Value()
	if err != nil {
		return nil, err
	}
	a, ok := prev.(*Object)
	if !ok {
		return new.Value, nil
	}
	next, err := new.Value.Value()
	if err != nil {
		return nil, err
	}
	b, ok := next.(*Object)
	if !ok {
		return new.Value, nil
	}
	merged, err := Merge(a, b)
	if err != nil {
		return nil, err
	}
	return Done(merged), nil
}

// Equal compares two values for "==", and fails where it would. Values of
// different types are never equal, except that integers and floats compare
// as the numbers they are.
func Equal(x, y Value) (bool, error) {
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
	c, err := Compare(x, y)
	return err == nil && c == 0, nil
}

// Compare orders two values for "<", answering a negative number when x sorts
// first, and fails where "<" would. Numbers and strings are the only things
// with an order.
func Compare(x, y Value) (int, error) {
	if a, ok := x.(Int); ok {
		if b, ok := y.(Int); ok {
			return cmp.Compare(a, b), nil
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

// member answers "x in y", which op names for its errors. A sequence holds x
// when one of its items equals it, a mapping when x names one of its keys,
// hidden ones included, and a string when x appears somewhere within it.
//
// Looking for x among the items of a sequence asks the same question "=="
// does, so x has to be a scalar. An item that is a collection is never equal
// to one, so it is passed over rather than reported.
func member(op token.Kind, pos token.Pos, x, y Value) (bool, error) {
	switch c := y.(type) {
	case *Array:
		if err := requireScalar(x); err != nil {
			return false, errorAt(pos, err)
		}
		for _, item := range c.Items() {
			v, err := item.Value.Value()
			if err != nil {
				return false, err
			}
			if same, err := Equal(x, v); err == nil && same {
				return true, nil
			}
		}
		return false, nil
	case *Object:
		k, ok := x.(String)
		if !ok {
			return false, errorf(pos, "%s needs a string to look for among the keys of a mapping, found %s",
				op, x.TypeName())
		}
		_, held := c.Lookup(string(k))
		return held, nil
	case String:
		s, ok := x.(String)
		if !ok {
			return false, errorf(pos, "%s needs a string to look for in a string, found %s", op, x.TypeName())
		}
		return strings.Contains(string(c), string(s)), nil
	}
	return false, errorf(pos, "%s needs a sequence, mapping or string to look in, found %s", op, y.TypeName())
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
