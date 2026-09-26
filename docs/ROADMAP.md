# Roadmap

Done so far: the restricted YAML parser, string interpolation, relative
references, hidden fields, context files, the `template` command, `local`
bindings, functions, the `size`, `empty` and `nullify` built-ins, the `??` and
`?.` operators, arithmetic, comparison and boolean operators,
`if`/`then`/`else`, sequence and mapping comprehensions, and the YAML, KYAML,
JSON, TOML and line-delimited output formats. What follows is the planned
order of the remaining work.

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

`render` belongs here rather than among the statements: it will be a built-in,
so the name stays an ordinary identifier until then.

## 2. `import`

Pull definitions out of another module. Like `local`, an `import` is a
statement of its own rather than an expression, and it comes in the same two
forms:

```yaml
import mydirectory.mymodule
import shared = mydirectory.mymodule
import {
  name1 = mydir.module1
  name2 = mydir.module2
}
```

The syntax and the resolution rules are specified in
[SPEC.md](SPEC.md#modules): a dotted path names a directory chain and the
`.yak` file at the end of it, relative to the importing file, and a `yak.mod`
and `yak.lock` pair remaps a module or its parent onto another directory or a
remote `https` or `git` location.

That splits into two pieces of work. The resolver turns a path into a source
file, which is where `yak.mod` and `yak.lock` are read, where a remote module
is fetched, and where a module cache keyed by resolved path belongs; the
per-document `docState` in `internal/eval/eval.go` is the natural place to
hang it, along with the cycle detection that importing across files needs. The
namespace is the smaller piece: a module evaluates to a value whose
definitions are read by field access, so `import` can bind one the way a
`local` binds anything else.

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
- `fmt` formats yak files following best practices.
- `mod` controls yak.mod and yak.lock files.

They slot into `internal/cli` beside `template`, and both can reuse
`internal/engine`.

## Smaller items

- Chained comprehension clauses. One `for` with an optional `if` is
  supported; jsonnet allows any number of them, which `parser.parseLoop`
  would have to return a slice of.
- Preserving flow style. Flow collections currently render as block
  collections; `ast.Mapping.Flow` and `ast.Sequence.Flow` record the original
  style if that becomes worth honouring.
- An `--indent` flag. `render.Options` already carries the width and every
  format reads it; nothing exposes it yet.
