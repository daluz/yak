// Package eval turns a yak syntax tree into values.
//
// Evaluation is lazy: a reference such as ".name" may point at a sibling
// declared further down the file, and ".." points at a mapping that is still
// being built, so every value sits behind a memoizing thunk that detects
// reference cycles.
package eval

import (
	"fmt"
	"strconv"
)

// Value is an evaluated yak value.
type Value interface {
	// TypeName returns the name used for the value's type in diagnostics.
	TypeName() string
}

// Null is the null value.
type Null struct{}

// Bool is a boolean value.
type Bool bool

// Int is an integer value.
type Int int64

// Float is a floating point value.
type Float float64

// String is a string value.
type String string

func (Null) TypeName() string   { return "null" }
func (Bool) TypeName() string   { return "boolean" }
func (Int) TypeName() string    { return "integer" }
func (Float) TypeName() string  { return "float" }
func (String) TypeName() string { return "string" }

// Field is one entry of an Object.
type Field struct {
	Name string
	// Hidden fields are written with "::". References can read them but the
	// renderer leaves them out of the output.
	Hidden bool
	Value  *Thunk
}

// Object is an ordered mapping.
type Object struct {
	fields []*Field
	index  map[string]int
}

// NewObject returns an empty mapping.
func NewObject() *Object {
	return &Object{index: map[string]int{}}
}

// TypeName implements Value.
func (*Object) TypeName() string { return "mapping" }

// Fields returns the mapping's entries in source order.
func (o *Object) Fields() []*Field { return o.fields }

// Len returns the number of entries, including hidden ones.
func (o *Object) Len() int { return len(o.fields) }

// Lookup returns the field with the given name.
func (o *Object) Lookup(name string) (*Field, bool) {
	i, ok := o.index[name]
	if !ok {
		return nil, false
	}
	return o.fields[i], true
}

// Names returns the field names in source order.
func (o *Object) Names() []string {
	names := make([]string, len(o.fields))
	for i, f := range o.fields {
		names[i] = f.Name
	}
	return names
}

// Set appends a field, or replaces the value of an existing one while keeping
// its original position.
func (o *Object) Set(name string, hidden bool, v *Thunk) {
	if i, ok := o.index[name]; ok {
		o.fields[i].Hidden = hidden
		o.fields[i].Value = v
		return
	}
	o.index[name] = len(o.fields)
	o.fields = append(o.fields, &Field{Name: name, Hidden: hidden, Value: v})
}

// reserve appends an unnamed slot, preserving source order while the key is
// still being evaluated.
func (o *Object) reserve(hidden bool, v *Thunk) *Field {
	f := &Field{Hidden: hidden, Value: v}
	o.fields = append(o.fields, f)
	return f
}

// bind assigns a name to a reserved slot.
func (o *Object) bind(f *Field, name string) error {
	if _, ok := o.index[name]; ok {
		return fmt.Errorf("duplicate key %q", name)
	}
	for i, existing := range o.fields {
		if existing == f {
			f.Name = name
			o.index[name] = i
			return nil
		}
	}
	return fmt.Errorf("internal error: unknown field slot for key %q", name)
}

// Array is an ordered sequence.
type Array struct {
	items []*Thunk
}

// NewArray returns a sequence holding the given items.
func NewArray(items []*Thunk) *Array { return &Array{items: items} }

// TypeName implements Value.
func (*Array) TypeName() string { return "sequence" }

// Items returns the sequence's items.
func (a *Array) Items() []*Thunk { return a.items }

// Len returns the number of items.
func (a *Array) Len() int { return len(a.items) }

// stringify converts a value to the text used when interpolating it into a
// string. Collections have no sensible textual form and are rejected.
func stringify(v Value) (string, error) {
	switch t := v.(type) {
	case Null:
		return "null", nil
	case Bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case Int:
		return strconv.FormatInt(int64(t), 10), nil
	case Float:
		return strconv.FormatFloat(float64(t), 'g', -1, 64), nil
	case String:
		return string(t), nil
	default:
		return "", fmt.Errorf("cannot interpolate a %s into a string", v.TypeName())
	}
}
