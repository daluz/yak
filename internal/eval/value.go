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

// Comments are the source comments written around an entry or an item, which
// the renderer writes out again around it.
type Comments struct {
	// Head holds the whole-line comments written above, each keeping the
	// "#" it was written with.
	Head []string
	// Line holds the comment written after it on the same line.
	Line string
	// Foot holds the comments that followed everything in the document.
	Foot []string
}

// Field is one entry of an Object.
type Field struct {
	Comments
	Name string
	// Hidden fields are written with "::". References can read them but the
	// renderer leaves them out of the output.
	Hidden bool
	// HideNull fields are written with "::?", which leaves the field out of
	// the output only when its value is null.
	HideNull bool
	Value    *Thunk
}

// Omitted reports whether the renderer leaves the field out of its output.
// Deciding that for a "::?" field means resolving its value, which is why
// this can fail.
func (f *Field) Omitted() (bool, error) {
	if f.Hidden {
		return true, nil
	}
	if !f.HideNull {
		return false, nil
	}
	v, err := f.Value.Value()
	if err != nil {
		return false, err
	}
	_, null := v.(Null)
	return null, nil
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
func (o *Object) reserve(c Comments, hidden, hideNull bool, v *Thunk) *Field {
	f := &Field{Comments: c, Hidden: hidden, HideNull: hideNull, Value: v}
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

// Elem is one item of an Array. It wraps the item's value so that the item
// can carry comments of its own, as a Field does.
type Elem struct {
	Comments
	Value *Thunk
}

// Item wraps an already computed value as a sequence item.
func Item(v Value) *Elem { return &Elem{Value: Done(v)} }

// Array is an ordered sequence.
type Array struct {
	items []*Elem
}

// NewArray returns a sequence holding the given items.
func NewArray(items []*Elem) *Array { return &Array{items: items} }

// TypeName implements Value.
func (*Array) TypeName() string { return "sequence" }

// Items returns the sequence's items.
func (a *Array) Items() []*Elem { return a.items }

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
