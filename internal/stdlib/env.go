package stdlib

import (
	"os"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/eval"
	"github.com/daluz/yak/internal/module"
)

// The env module reads environment variables, which can change from one
// rendering to the next, so strict mode refuses it.
func init() {
	module.Register(module.BuiltinModule{Path: "std.env", Nondeterministic: true, Builtins: []*eval.Builtin{
		builtin("get", append(params("name"), optional("default", &ast.Null{})), getEnv),
	}})
}

// getEnv answers the value of the variable name, or default when it is not
// set. A variable set to "" answers "".
func getEnv(c call) (eval.Value, error) {
	name, err := c.str(0)
	if err != nil {
		return nil, err
	}
	if v, ok := os.LookupEnv(name); ok {
		return eval.String(v), nil
	}
	return c.value(1)
}
