package eval

import "github.com/daluz/yak/internal/ast"

// rebinds reports whether binds bring any name into scope more than once.
func rebinds(binds []*ast.Binding) bool {
	seen := map[string]bool{}
	again := false
	note := func(name string) {
		again = again || seen[name]
		seen[name] = true
	}
	for _, b := range binds {
		if b.Pattern == nil {
			note(b.Name)
			continue
		}
		b.Pattern.Walk(func(name *ast.Pattern, _ []int) { note(name.Name) })
	}
	return again
}

// bind adds to frame the names b brings into scope, with its value evaluated
// in env. Each name carries the comments written on the binding.
func bind(b *ast.Binding, env *Env, frame map[string]*Thunk) {
	value := &Thunk{node: b.Value, env: env, pos: b.Value.Pos()}
	if b.Pattern == nil {
		value.comments = comments(b.Comments)
		frame[b.Name] = value
		return
	}
	destructure(b.Pattern, value, env, comments(b.Comments), frame)
}

// destructure adds to frame the names pat binds in the value of t, each
// carrying c. The names share t, and the first of them to be read checks the
// whole of its value against the pattern, so a value that does not fit is
// reported the same way whichever name is read. env is only where the names
// are reported from should they form a cycle.
func destructure(pat *ast.Pattern, t *Thunk, env *Env, c Comments, frame map[string]*Thunk) {
	checked := &Thunk{env: env, pos: pat.Pos(), fn: func() (Value, error) {
		return nil, fit(pat, t, "")
	}}
	pat.Walk(func(name *ast.Pattern, path []int) {
		frame[name.Name] = &Thunk{env: env, pos: name.Pos(), comments: c, fn: func() (Value, error) {
			if _, err := checked.Value(); err != nil {
				return nil, err
			}
			return part(t, path).Value()
		}}
	})
}

// fit checks that the value of t has the shape pat destructures. The items a
// nested pattern destructures are read to see their shape too; the rest stay
// lazy. where is added to a mismatch to say which value did not fit.
func fit(pat *ast.Pattern, t *Thunk, where string) error {
	if pat.Items == nil {
		return nil
	}
	v, err := t.Value()
	if err != nil {
		return err
	}
	arr, ok := v.(*Array)
	if !ok {
		return errorf(pat.Pos(), "the pattern needs a sequence of %s, found %s%s",
			plural(len(pat.Items), "item"), v.TypeName(), where)
	}
	if arr.Len() != len(pat.Items) {
		return errorf(pat.Pos(), "the pattern needs a sequence of %s, found %s%s",
			plural(len(pat.Items), "item"), plural(arr.Len(), "item"), where)
	}
	for i, item := range pat.Items {
		if err := fit(item, arr.Items()[i].Value, where); err != nil {
			return err
		}
	}
	return nil
}

// part answers the thunk of the item that path leads to from t, whose shape
// fit has already checked.
func part(t *Thunk, path []int) *Thunk {
	for _, i := range path {
		v, _ := t.Value()
		t = v.(*Array).Items()[i].Value
	}
	return t
}
