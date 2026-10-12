package stdlib

import (
	"crypto/rand"
	"fmt"
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
	return lo + eval.Int(mathrand.Int64N(int64(hi-lo)+1)), nil
}

// randfloat is rand, which answers a float from 0 up to but not including 1.
func randfloat(call) (eval.Value, error) {
	return eval.Float(mathrand.Float64()), nil
}

func choice(c call) (eval.Value, error) {
	_, values, err := c.seq(0)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, c.errorf(0, "needs a sequence with an item in it, found []")
	}
	return values[mathrand.IntN(len(values))], nil
}

// shuffle answers the items in a random order, each keeping its comments.
func shuffle(c call) (eval.Value, error) {
	a, _, err := c.seq(0)
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
