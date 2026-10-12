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
		builtin("sha1", valueParam, digest(func(b []byte) []byte { s := sha1.Sum(b); return s[:] })),
		builtin("sha256", valueParam, digest(func(b []byte) []byte { s := sha256.Sum256(b); return s[:] })),
		builtin("sha512", valueParam, digest(func(b []byte) []byte { s := sha512.Sum512(b); return s[:] })),
	}})
}

func digest(sum func([]byte) []byte) func(call) (eval.Value, error) {
	return mapString(func(s string) string { return hex.EncodeToString(sum([]byte(s))) })
}
