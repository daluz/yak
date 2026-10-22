package stdlib

import (
	"crypto/rand"
	"fmt"
	"math"
	mathrand "math/rand/v2"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The random module answers differently on every rendering, so strict mode
// refuses it.
func init() {
	module.Register(module.BuiltinModule{Path: "std.random", Nondeterministic: true, Builtins: []*eval.Builtin{
		builtin("randint", params("min", "max"), randint),
		builtin("rand", nil, randfloat),
		builtin("choice", valuesParam, choice),
		builtin("shuffle", valuesParam, shuffle),
		builtin("uuid", nil, uuid),
	}})
}

// randint answers an integer from min to max, both included.
func randint(c call) (eval.Value, error) {
	lo, err := c.integer(0)
	if err != nil {
		return nil, err
	}
	hi, err := c.integer(1)
	if err != nil {
		return nil, err
	}
	if hi < lo {
		return nil, c.errorf(1, "needs a max of at least min %d, found %d", lo, hi)
	}
	span := uint64(hi) - uint64(lo)
	if span == math.MaxUint64 {
		return eval.Int(mathrand.Uint64()), nil
	}
	return eval.Int(uint64(lo) + mathrand.Uint64N(span+1)), nil
}

// randfloat is rand, which answers a float from 0 up to but not including 1.
func randfloat(call) (eval.Value, error) {
	return eval.Float(mathrand.Float64()), nil
}

func choice(c call) (eval.Value, error) {
	a, err := c.array(0)
	if err != nil {
		return nil, err
	}
	if a.Len() == 0 {
		return nil, c.errorf(0, "needs a sequence with an item in it, found []")
	}
	return a.Items()[mathrand.IntN(a.Len())].Value.Value()
}

// shuffle answers the items in a random order, each keeping its comments.
func shuffle(c call) (eval.Value, error) {
	a, err := c.array(0)
	if err != nil {
		return nil, err
	}
	items := append([]*eval.Elem(nil), a.Items()...)
	mathrand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	return eval.NewArray(items), nil
}

// uuid answers a random (version 4) UUID.
func uuid(call) (eval.Value, error) {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return eval.String(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])), nil
}
