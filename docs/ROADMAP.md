# Roadmap

Done so far: the restricted YAML parser, string interpolation, relative
references, hidden fields, context files, the `render` command, `let`
bindings, functions named and anonymous, the `size`, `empty`, `nempty`,
`prune` and `render` built-ins and those on sequences and mappings, the `std.strings`,
`std.json`, `std.yaml`, `std.base64`, `std.hash`, `std.regex`, `std.math`,
`std.path`, `std.files`, `std.gotemplate` and `std.random` modules, strict
mode, the
`??` and
`?.` operators, arithmetic, comparison and boolean operators, the `<<` merge
in both of its forms,
`if`/`then`/`else`, sequence and mapping comprehensions, `import` of local
modules and `.yakl` libraries, packages defined by `yak.pkg` with local,
git and OCI dependencies, the `package` command with `init`, `add`, `remove`,
`sync`, `build` and `push`, `yak.lock`, rendering a directory or a remote
package, and the YAML, KYAML,
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
a `let` shadows it. Built-ins are grouped into built-in modules under
`internal/stdlib`, each of which hands a `module.BuiltinModule` to
`module.Register` from its `init`. The root module, in `root.go`, has no path,
so its members are bare names; `size`, `empty`, `nempty`, `prune` and `render` live
there, and
`seqs.go` and `maps.go` register the operations on sequences and mappings into
it as well.

The rest of the library is namespaced rather than bare, so that adding a
function cannot take a name a template is already using. Each module is a
built-in module registered under a dotted path beginning with `std`, which
`import std.name` binds as a mapping of its built-ins; the `std` segment is
one that no file or `yak.pkg` can take. Each module has a file of its own,
and names that take more than one word are camelCase. A built-in that takes a function calls it with `eval.Call`.

What remains is formatting.

## 2. `schema`

Validation and type definitions, including default values.

```yaml
schema {
  name: string
  replicas: int = 1
}
```

Schemas will also bring back tags, which the lexer currently rejects with a
message that says so.

## 3. Remaining commands

`render` and `package` are in; [CLI.md](CLI.md) documents them.

- `check` checks inputs and outputs against a schema.
- `fmt` formats yak files following best practices.

They slot into `internal/cli` beside `render`, and both can reuse
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
- `$super`, the mapping on the left of a `<<` read from the one on the right.
  The name is reserved and `parser.parseBuiltin` rejects it for now. What it
  needs is a decision first: `<<` is lexical today, so a merged entry reads
  the mapping it was written in, and a `$super` that matched jsonnet's would
  have to be bound when the merge happens rather than where it is written.
- A built-in for merges that `<<` does not cover, such as a shallow one or a
  sequence that appends rather than being replaced. `<<` is the one merge the
  language spells, and the rest belong in the standard library where the
  choice can be named.
