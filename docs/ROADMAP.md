# Roadmap

Done so far: the restricted YAML parser, string interpolation, relative
references, hidden fields, context files, the `template` command, `local`
bindings, functions, the `??` and `?.` operators, comparison and boolean
operators, `if`/`then`/`else`, sequence and mapping comprehensions, and the
YAML, KYAML, JSON, TOML and line-delimited output formats. What follows is the
planned order of the remaining work.

Everything listed here is already reserved in the language. Using one of these
keywords today produces an explicit "not implemented yet" error pointing at the
right position, so no program silently means something different once the
feature lands.

## 1. Standard library

Built-in functions for string manipulation, collection handling, encoding, and
formatting. These want a namespace that cannot collide with user bindings.

Functions are in, so the call machinery already exists: a built-in needs a
value that `eval.evalCall` accepts beside `*eval.Function`, and the same
argument matching that `Function.arguments` does.

## 2. `import`

Pull definitions out of another `.yak` or `.libyak` file.

```yaml
local shared = import "shared.libyak"
```

This is where `.libyak` becomes meaningful: `yak template` already refuses to
render one directly.

Imports need a resolver with a search path, a cache keyed by resolved path, and
cycle detection across files. The per-document `docState` in
`internal/eval/eval.go` is the natural place to hang that cache.

## 3. `schema`

Validation and type definitions, including default values.

```yaml
schema {
  name: string
  replicas: int = 1
}
```

Schemas will also bring back tags, which the lexer currently rejects with a
message that says so.

## 4. Remaining commands

- `build` renders a multi-file project rather than a single template.
- `validate` checks inputs and outputs against a schema.

Both slot into `internal/cli` beside `template`, and both can reuse
`internal/engine`.

## Smaller items

- Arithmetic. The comparison and boolean operators are in, and
  `parser.precedence` is the table to add `+`, `-`, `*`, `/` and `%` to.
  Note that `-` is allowed inside identifiers, so binary operators require
  surrounding whitespace.
- Chained comprehension clauses. One `for` with an optional `if` is
  supported; jsonnet allows any number of them, which `parser.parseLoop`
  would have to return a slice of.
- Preserving flow style. Flow collections currently render as block
  collections; `ast.Mapping.Flow` and `ast.Sequence.Flow` record the original
  style if that becomes worth honouring.
- An `--indent` flag. `render.Options` already carries the width and every
  format reads it; nothing exposes it yet.
