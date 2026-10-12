package stdlib

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"

	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The hash module digests a string into lowercase hexadecimal, which is how
// a checksum of rendered configuration is usually written.
func init() {
	module.Register(module.BuiltinModule{Path: "std.hash", Builtins: []*eval.Builtin{
		{Name: "sha1", Params: valueParam, Fn: digest("sha1", func(b []byte) []byte { s := sha1.Sum(b); return s[:] })},
		{Name: "sha256", Params: valueParam, Fn: digest("sha256", func(b []byte) []byte { s := sha256.Sum256(b); return s[:] })},
		{Name: "sha512", Params: valueParam, Fn: digest("sha512", func(b []byte) []byte { s := sha512.Sum512(b); return s[:] })},
	}})
}

func digest(name string, sum func([]byte) []byte) func([]*eval.Thunk) (eval.Value, error) {
	return mapString(name, func(s string) string { return hex.EncodeToString(sum([]byte(s))) })
}
