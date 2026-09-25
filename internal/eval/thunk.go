package eval

import (
	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

type thunkState int

const (
	pending thunkState = iota
	running
	resolved
)

// Thunk is a value that is computed on first use and remembered afterwards.
type Thunk struct {
	node  ast.Node
	env   *Env
	pos   token.Pos
	state thunkState
	val   Value
	err   error
}

// Done wraps an already computed value.
func Done(v Value) *Thunk {
	return &Thunk{state: resolved, val: v}
}

// Pos reports where the thunk's expression starts.
func (t *Thunk) Pos() token.Pos { return t.pos }

// Value evaluates the thunk, or returns the previously computed result.
func (t *Thunk) Value() (Value, error) {
	switch t.state {
	case resolved:
		return t.val, t.err
	case running:
		return nil, t.cycleError()
	}

	t.state = running
	ds := t.env.doc
	ds.stack = append(ds.stack, t.pos)

	v, err := evalNode(t.node, t.env)

	ds.stack = ds.stack[:len(ds.stack)-1]
	t.state = resolved
	t.val, t.err = v, err
	return v, err
}

// cycleError describes a reference cycle as the chain of positions that led
// back to this thunk.
func (t *Thunk) cycleError() error {
	stack := t.env.doc.stack
	from := 0
	for i, p := range stack {
		if p == t.pos {
			from = i
			break
		}
	}
	trace := append([]token.Pos(nil), stack[from:]...)
	trace = append(trace, t.pos)
	return &Error{
		Pos:        t.pos,
		Msg:        "circular reference: this value depends on itself",
		Trace:      trace,
		TraceTitle: "the cycle passes through",
	}
}

// Force resolves a value and everything reachable from it, so that errors
// surface before any output is written.
//
// A value may legally refer to a collection that encloses it, which resolves
// fine but cannot be written out; that is reported here rather than as a
// stack overflow in the renderer.
func Force(v Value) error {
	return force(v, map[Value]bool{})
}

func force(v Value, active map[Value]bool) error {
	switch t := v.(type) {
	case *Object:
		if active[t] {
			return &Error{Msg: "mapping contains itself and cannot be rendered"}
		}
		active[t] = true
		defer delete(active, t)
		for _, f := range t.Fields() {
			inner, err := f.Value.Value()
			if err != nil {
				return err
			}
			if err := force(inner, active); err != nil {
				return annotate(err, f.Value.pos)
			}
		}
	case *Array:
		if active[t] {
			return &Error{Msg: "sequence contains itself and cannot be rendered"}
		}
		active[t] = true
		defer delete(active, t)
		for _, item := range t.Items() {
			inner, err := item.Value()
			if err != nil {
				return err
			}
			if err := force(inner, active); err != nil {
				return annotate(err, item.pos)
			}
		}
	}
	return nil
}

// annotate fills in a position for errors raised while walking a structure,
// which have no expression of their own to point at.
func annotate(err error, pos token.Pos) error {
	e, ok := err.(*Error)
	if !ok || e.Pos.IsValid() {
		return err
	}
	e.Pos = pos
	return e
}
