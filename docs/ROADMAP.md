# Roadmap

Phase one is done: the restricted YAML parser, string interpolation, relative
references, hidden fields, context files, and the `template` command. What
follows is the planned order of the remaining work.

Everything listed here is already reserved in the language. Using one of these
keywords today produces an explicit "not implemented yet" error pointing at the
right position, so no program silently means something different once the
feature lands.

## 1. `local` bindings

Variables, functions, and blocks, all introduced by `local`.

```yaml
local name = "web"
local url(host, port) = "https://${host}:${port}"

local {
  region = "us-east-1"
  zone = "${region}a"
}

service: name
endpoint: url($$.host, 8080)
```

Notes for the implementation:

- `ast.Ident` already exists and is the single place where a bare identifier
  fails to resolve, so binding lookup slots in there.
- `Env` already threads a lexical environment through evaluation; bindings add
  a scope chain alongside the existing `self` chain.
- Function calls need a `Call` node and a postfix `(` rule in
  `internal/parser/expr.go`, next to the existing field and index rules.
- The `=` token is already lexed for this purpose.

## 2. Standard library

Built-in functions for string manipulation, collection handling, encoding, and
formatting. These want a namespace that cannot collide with user bindings.

## 3. `import`

Pull definitions out of another `.yak` or `.libyak` file.

```yaml
local shared = import "shared.libyak"
```

This is where `.libyak` becomes meaningful: `yak template` already refuses to
render one directly.

Imports need a resolver with a search path, a cache keyed by resolved path, and
cycle detection across files. The per-document `docState` in
`internal/eval/eval.go` is the natural place to hang that cache.

## 4. `schema`

Validation and type definitions, including default values.

```yaml
schema {
  name: string
  replicas: int = 1
}
```

Schemas will also bring back tags, which the lexer currently rejects with a
message that says so.

## 5. Remaining commands

- `build` renders a multi-file project rather than a single template.
- `validate` checks inputs and outputs against a schema.

Both slot into `internal/cli` beside `template`, and both can reuse
`internal/engine`.

## Smaller items

- Operators. Arithmetic and comparison need a precedence table in the Pratt
  parser. Note that `-` is currently allowed inside identifiers, so binary
  operators will require surrounding whitespace.
- `--format json`. `render.Options` already carries a format enum and
  `render.Documents` already branches on it.
- Preserving flow style. Flow collections currently render as block
  collections; `ast.Mapping.Flow` and `ast.Sequence.Flow` record the original
  style if that becomes worth honouring.
