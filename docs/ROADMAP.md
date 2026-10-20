# Roadmap

Everything [SPEC.md](SPEC.md), [STDLIB.md](STDLIB.md) and [CLI.md](CLI.md)
describe is implemented. What follows is the planned order of the remaining
work.

## 1. `fmt`

`yak fmt` formats yak files following best practices. It slots into
`internal/cli` beside `render`, and can reuse `internal/engine`.

## 2. More `schema import` formats

`schema import` reads Kubernetes CustomResourceDefinitions. JSON Schema and
OpenAPI v3 documents are to follow, each as a value of `--format` that
`schemaimport.Detect` recognizes; `internal/schemaimport` translates an
OpenAPI schema node already, and `crd.go` is what is CRD-specific.

## Smaller items

- Chained comprehension clauses. One `for` with an optional `if` is
  supported; jsonnet allows any number of them, which `parser.parseLoop`
  would have to return a slice of.
- Preserving flow style. Flow collections currently render as block
  collections; `ast.Mapping.Flow` and `ast.Sequence.Flow` record the original
  style if that becomes worth honouring.
- An `--indent` flag. `render.Options` already carries the width and every
  format reads it; nothing exposes it yet.
- A built-in for merges that `<<` does not cover, such as a shallow one or a
  sequence that appends rather than being replaced. `<<` is the one merge the
  language spells, and the rest belong in the standard library where the
  choice can be named.
