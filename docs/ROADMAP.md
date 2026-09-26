# Roadmap

Done so far: the restricted YAML parser, string interpolation, relative
references, hidden fields, context files, the `template` command, `local`
bindings, functions, the `size`, `empty` and `nullify` built-ins, the `??` and
`?.` operators, comparison and boolean operators, `if`/`then`/`else`, sequence
and mapping comprehensions, and the YAML, KYAML, JSON, TOML and line-delimited
output formats. What follows is the planned order of the remaining work.

Everything listed here is already reserved in the language. Using one of these
keywords today produces an explicit "not implemented yet" error pointing at the
right position, so no program silently means something different once the
feature lands.

## 1. Standard library modules

The built-ins are in: `eval.Builtin` is a value that `evalCall` accepts beside
`*eval.Function`, it goes through the same `matchArguments` that a user
function does, and `evalIdent` finds one only after every binding in scope, so
a `local` shadows it. `size`, `empty` and `nullify` are the three that live
there.

What remains is the larger part of the library — string manipulation,
encoding, and formatting — which will be namespaced rather than bare, so that
adding a function cannot take a name a template is already using. A namespace
can be an `*eval.Object` of built-ins resolved outside the document, since
field access already reads a function out of a mapping; what it needs is a
name that a template cannot bind.

## 2. `import`

Pull definitions out of another `.yak` file.

```yaml
local shared = import "shared.yak"
```

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
